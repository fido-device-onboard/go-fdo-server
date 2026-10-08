// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package serviceinfo

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"testing"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
)

// TestModuleStateMachinesMapOperations tests that map operations are properly protected
// This test verifies the fix for the concurrent map access race condition
func TestModuleStateMachinesMapOperations(t *testing.T) {
	msm := &ModuleStateMachines{
		states: make(map[string]*moduleStateMachineState),
	}

	// Test concurrent reads, writes, and deletes
	numGoroutines := 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			token := string(rune('A' + (id % 26))) // Reuse tokens to increase contention

			// Simulate concurrent map operations that happen during TO2 protocol
			// These operations previously caused "concurrent map read and map write" panics

			// Write to states map
			msm.mu.Lock()
			msm.states[token] = &moduleStateMachineState{
				Name: "test-module",
			}
			msm.mu.Unlock()

			// Read from states map
			msm.mu.RLock()
			_, exists := msm.states[token]
			msm.mu.RUnlock()

			if !exists {
				// This is expected due to concurrent access - another goroutine might have deleted it
				return
			}

			// Read again (to increase read concurrency)
			msm.mu.RLock()
			_, _ = msm.states[token]
			msm.mu.RUnlock()

			// Delete from states map
			msm.mu.Lock()
			delete(msm.states, token)
			msm.mu.Unlock()
		}(i)
	}

	wg.Wait()
	// If we reach here without panic, the mutex is working correctly
	t.Log("Concurrent map access completed successfully - no data race detected")
}

// TestModuleStateMachinesConcurrentReadWrite specifically tests read-write races
func TestModuleStateMachinesConcurrentReadWrite(t *testing.T) {
	msm := &ModuleStateMachines{
		states: make(map[string]*moduleStateMachineState),
	}

	const numReaders = 20
	const numWriters = 10
	var wg sync.WaitGroup

	// Spawn multiple readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				msm.mu.RLock()
				_ = msm.states["shared-token"]
				msm.mu.RUnlock()
			}
		}()
	}

	// Spawn multiple writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				msm.mu.Lock()
				msm.states["shared-token"] = &moduleStateMachineState{
					Name: "test",
				}
				msm.mu.Unlock()
			}
		}()
	}

	wg.Wait()
	t.Log("Concurrent read-write operations completed successfully")
}

// TestModuleStateMachinesAPIUnderRace drives the exported methods concurrently,
// rather than poking the states map directly as the tests above do. Reading
// Name and Impl outside a lock while another request advances the same session
// is a data race the map mutex alone does not prevent, so this is what catches
// a regression in the per-session locking.
func TestModuleStateMachinesAPIUnderRace(t *testing.T) {
	const (
		numTokens  = 8
		numReaders = 16
		iterations = 50
	)

	msm := &ModuleStateMachines{
		OwnerState: &state.OwnerState{Token: &state.TokenService{}},
		states:     make(map[string]*moduleStateMachineState),
	}

	tokens := make([]string, numTokens)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("token-%d", i)
		// Seed a session the way NextModule would, with an iterator that
		// yields without touching a database.
		next, stop := iter.Pull2(func(yield func(string, serviceinfo.OwnerModule) bool) {
			for n := 0; yield(fmt.Sprintf("module-%d", n), nil); n++ { //nolint:revive // drives the iterator
			}
		})
		msm.states[tokens[i]] = &moduleStateMachineState{Next: next, Stop: stop}
	}

	ctxFor := func(token string) context.Context {
		return msm.OwnerState.Token.TokenContext(context.Background(), token)
	}

	var wg sync.WaitGroup

	// Readers call Module while writers advance the same sessions.
	for _, token := range tokens {
		for range numReaders {
			wg.Add(1)
			go func(token string) {
				defer wg.Done()
				for range iterations {
					// Either outcome is valid: the session may have been
					// cleaned up by the goroutines below.
					_, _, _ = msm.Module(ctxFor(token))
				}
			}(token)
		}

		// Advance the session, mirroring what NextModule does under the
		// per-session lock. NextModule itself needs a database for devmod.
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			for range iterations {
				module, ok := msm.load(token)
				if !ok {
					return
				}
				module.mu.Lock()
				module.Name, module.Impl, _ = module.Next()
				module.mu.Unlock()
			}
		}(token)
	}

	// Cleanups race the readers and writers above. CleanupModules needs no
	// database, so it runs for real.
	for _, token := range tokens[:numTokens/2] {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			msm.CleanupModules(ctxFor(token))
		}(token)
	}

	wg.Wait()

	// Every cleaned up session must be gone from the map.
	for _, token := range tokens[:numTokens/2] {
		if _, ok := msm.load(token); ok {
			t.Errorf("session %q still registered after CleanupModules", token)
		}
	}

	// Surviving sessions must still be usable.
	for _, token := range tokens[numTokens/2:] {
		if _, _, err := msm.Module(ctxFor(token)); err != nil {
			t.Errorf("Module(%q) after concurrent access: %v", token, err)
		}
	}

	// Stop what the test seeded and the cleanups did not take.
	for _, token := range tokens[numTokens/2:] {
		msm.CleanupModules(ctxFor(token))
	}
}
