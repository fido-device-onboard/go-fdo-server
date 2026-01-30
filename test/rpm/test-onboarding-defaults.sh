#!/bin/bash

set -euo pipefail

# Source the common CI test first
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-onboarding.sh"

# Test the default configuration and certificate generation provided by the RPMs.
# Force this by setting the certificate and configuration functions to no-ops.

# TODO: get the Device CA certificate from manufacturer's config
# with 'yq'
device_ca_crt="/etc/pki/go-fdo-server/device-ca-example.crt"

generate_service_certs() {
  return 0
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
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
