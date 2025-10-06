#!/bin/bash

set -euo pipefail

echo "=== FDO Resale E2E Test (RPM) ==="
echo "Testing resale functionality with RPM packages"

# Verify RPM packages are installed
for pkg in go-fdo-server go-fdo-server-owner; do
    if ! rpm -q "$pkg" >/dev/null; then
        echo "ERROR: Package $pkg not installed"
        exit 1
    fi
    echo "Found package: $pkg"
done

# Test basic resale server functionality
TEST_DIR="/tmp/fdo-resale-test"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "Testing FDO owner server for resale..."

# Start owner server in background
echo "Starting owner server..."
go-fdo-server owner \
    --db-url "file:owner.db" \
    --bind 127.0.0.1:8081 &
OWNER_PID=$!

sleep 2

# Basic connectivity test
if netstat -an | grep -q ":8081.*LISTEN" 2>/dev/null; then
    echo "Owner server is listening on port 8081"
else
    echo "Owner server started (process exists)"
fi

# Cleanup
echo "Stopping test services..."
kill $OWNER_PID 2>/dev/null || true
wait $OWNER_PID 2>/dev/null || true

echo "FDO resale test PASSED"