#!/bin/bash

set -euo pipefail

# Source the common CI test first (defines certs_dir via CI utils)
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)/test-onboarding.sh"

# Force all services to use HTTPS for this test
manufacturer_protocol=https
manufacturer_url="${manufacturer_protocol}://${manufacturer_service}"
manufacturer_health_url="${manufacturer_url}/health"
rendezvous_protocol=https
rendezvous_url="${rendezvous_protocol}://${rendezvous_service}"
rendezvous_health_url="${rendezvous_url}/health"
owner_protocol=https
owner_url="${owner_protocol}://${owner_service}"
owner_health_url="${owner_url}/health"

# Enable HTTPS by including https certs/keys in the server's
# configuration file.

configure_service_manufacturer() {
  log_info "Generating manufacturer HTTPS configuration"
  sudo cp "${manufacturer_https_key}" "${rpm_manufacturer_https_key}"
  sudo cp "${manufacturer_https_crt}" "${rpm_manufacturer_https_crt}"
  sudo chown ${rpm_manufacturer_user}:${rpm_group} "${rpm_manufacturer_https_key}" "${rpm_manufacturer_https_crt}"
  generate_manufacturer_config
}

configure_service_rendezvous() {
  log_info "Generating rendezvous HTTPS configuration"
  sudo cp "${rendezvous_https_key}" "${rpm_rendezvous_https_key}"
  sudo cp "${rendezvous_https_crt}" "${rpm_rendezvous_https_crt}"
  sudo chown ${rpm_rendezvous_user}:${rpm_group} "${rpm_rendezvous_https_key}" "${rpm_rendezvous_https_crt}"
  generate_rendezvous_config
}

configure_service_owner() {
  log_info "Generating owner HTTPS configuration"
  if [ "${rendezvous_protocol}" = "https" ]; then
    rpm_owner_to0_insecure_tls="true"
  fi
  sudo cp "${owner_https_key}" "${rpm_owner_https_key}"
  sudo cp "${owner_https_crt}" "${rpm_owner_https_crt}"
  sudo chown ${rpm_owner_user}:${rpm_group} "${rpm_owner_https_key}" "${rpm_owner_https_crt}"
  generate_owner_config
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test; cleanup; }

