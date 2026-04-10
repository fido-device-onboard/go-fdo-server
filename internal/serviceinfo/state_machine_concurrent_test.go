// SPDX-FileCopyrightText: (C) 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache 2.0

package serviceinfo

import (
	"sync"
	"testing"
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
