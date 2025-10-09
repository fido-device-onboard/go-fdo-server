#!/bin/bash

set -euox pipefail

# E2E Test Script for go-fdo-server
# This script tests the FDO onboarding workflow with local services.
# 127.0.0.1 usage is intentional for testing local FDO server instances.
# devskim: ignore DS137138

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

function error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

function warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

BASE_DIR="$(pwd)/fdo-tmt-test"
LISTEN_IP="127.0.0.1"
MANUFACTURER_PORT=8038
RENDEZVOUS_PORT=8041
OWNER_PORT=8043
DEVICE_GUID=""

cleanup() {
    info "Cleaning up..."
    systemctl stop go-fdo-server-manufacturer.service go-fdo-server-rendezvous.service go-fdo-server-owner.service || true
    rm -rf "$BASE_DIR" || true
}
trap cleanup EXIT

# Setup and start servers
info "Setting up test environment..."
systemctl stop go-fdo-server-manufacturer.service go-fdo-server-rendezvous.service go-fdo-server-owner.service 2>/dev/null || true
rm -rf "$BASE_DIR" && mkdir -p "$BASE_DIR"
mkdir -p /var/lib/go-fdo-server-manufacturer /var/lib/go-fdo-server-rendezvous /var/lib/go-fdo-server-owner

info "Starting FDO servers..."
for service in rendezvous manufacturer owner; do
    info "Starting $service server..."
    systemctl start go-fdo-server-$service.service || {
        error "Failed to start $service server"
        journalctl -u go-fdo-server-$service.service --no-pager
        exit 1
    }
    if ! systemctl is-active --quiet go-fdo-server-$service.service; then
        error "$service server is not active"
        journalctl -u go-fdo-server-$service.service --no-pager
        exit 1
    fi
done
sleep 5

# Wait for servers and verify client
info "Waiting for servers to be ready..."
for port in $RENDEZVOUS_PORT $MANUFACTURER_PORT $OWNER_PORT; do
    for i in {1..30}; do
        curl --fail --silent "http://$LISTEN_IP:$port/health" >/dev/null 2>&1 && break
        if [ $i -eq 30 ]; then
            error "Server on port $port failed to respond after 30 seconds"
            exit 1
        fi
        sleep 1
    done
done

info "Verifying go-fdo-client installation..."
if ! command -v go-fdo-client >/dev/null; then
    error "go-fdo-client not found in PATH"
    exit 1
fi

# Configure rendezvous
info "Configuring rendezvous information..."
if ! curl --fail --silent --show-error -X POST "http://$LISTEN_IP:$MANUFACTURER_PORT/api/v1/rvinfo" \
    -H "Content-Type: application/json" \
    -d '[{"ip":"'$LISTEN_IP'","device_port":"'$RENDEZVOUS_PORT'","owner_port":"'$RENDEZVOUS_PORT'","protocol":"http"}]'; then
    error "Failed to configure rendezvous information"
    exit 1
fi

# Device initialization
info "Running Device Initialization (DI)..."
if ! go-fdo-client device-init "http://$LISTEN_IP:$MANUFACTURER_PORT" --blob="$BASE_DIR/device_credentials" --key=ec256 --debug; then
    error "Device initialization failed"
    exit 1
fi

DEVICE_GUID=$(go-fdo-client print --blob="$BASE_DIR/device_credentials" | grep "GUID" | awk '{print $2}')
if [ -z "$DEVICE_GUID" ]; then
    error "Failed to extract device GUID"
    exit 1
fi
info "Device GUID: $DEVICE_GUID"

# Configure owner redirect
info "Configuring owner redirect..."
if ! curl --fail --silent --show-error -X POST "http://$LISTEN_IP:$OWNER_PORT/api/v1/owner/redirect" \
    -H "Content-Type: application/json" \
    -d '[{"dns":"owner","port":"'$OWNER_PORT'","protocol":"http","ip":"'$LISTEN_IP'"}]'; then
    error "Failed to configure owner redirect"
    exit 1
fi

# Voucher transfer and onboarding
info "Downloading ownership voucher..."
if ! curl --fail --silent --show-error "http://$LISTEN_IP:$MANUFACTURER_PORT/api/v1/vouchers/$DEVICE_GUID" -o "$BASE_DIR/voucher.pem"; then
    error "Failed to download voucher"
    exit 1
fi

info "Uploading voucher to Owner server..."
if ! curl --fail --silent --show-error --request POST --data-binary @"$BASE_DIR/voucher.pem" "http://$LISTEN_IP:$OWNER_PORT/api/v1/owner/vouchers"; then
    error "Failed to upload voucher to owner"
    exit 1
fi

info "Running TO0 protocol..."
if ! curl --fail --silent --show-error "http://$LISTEN_IP:$OWNER_PORT/api/v1/to0/$DEVICE_GUID"; then
    error "TO0 protocol failed"
    exit 1
fi

# Device onboarding
info "Running device onboarding (TO1/TO2)..."
if ! go-fdo-client onboard --blob="$BASE_DIR/device_credentials" --key=ec256 --kex=ECDH256 --debug | tee "$BASE_DIR/onboarding.log"; then
    error "Device onboarding failed"
    exit 1
fi

# Verify completion
info "Verifying onboarding completion..."
if ! grep -q 'FIDO Device Onboard Complete' "$BASE_DIR/onboarding.log"; then
    error "Onboarding did not complete successfully"
    warn "Onboarding log contents:"
    cat "$BASE_DIR/onboarding.log"
    exit 1
fi

# Check service logs for errors
info "Checking service logs for errors..."
journalctl -u go-fdo-server-manufacturer.service --no-pager | tail -20
journalctl -u go-fdo-server-rendezvous.service --no-pager | tail -20
journalctl -u go-fdo-server-owner.service --no-pager | tail -20

# Success
info "======================================="
info "Go FDO Server E2E Test PASSED"
info "======================================="
info "✓ FDO server services started successfully"
info "✓ Rendezvous info configured"
info "✓ Device initialization (DI) completed"
info "✓ Device GUID extracted: ${DEVICE_GUID}"
info "✓ Owner redirect configured"
info "✓ Ownership voucher transferred"
info "✓ TO0 protocol completed"
info "✓ Device onboarding (TO1/TO2) completed"
info "✓ Full end-to-end FDO workflow validated"
info "======================================="