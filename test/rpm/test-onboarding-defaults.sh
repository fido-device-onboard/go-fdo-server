#!/bin/bash

set -euo pipefail

# Source the common CI test first
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)/test-onboarding.sh"

# Test the default configuration provided by the RPMs.
# Generate certificates using the helper script, then use default configurations.

generate_service_certs() {
  sudo /usr/libexec/go-fdo-server/generate-go-fdo-server-certs.sh
}

configure_service_manufacturer() {
  return 0
}

configure_service_rendezvous() {
  return 0
}

configure_service_owner() {
  return 0
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test; cleanup; }
