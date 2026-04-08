#! /usr/bin/env bash
# RV bypass test: Device skips TO1 by getting Owner address directly from voucher (TO0 not needed)

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

run_test() {
  # Override services array to exclude rendezvous (not needed with RV bypass)
  services=("${manufacturer_service_name}" "${owner_service_name}")

  log_info "Setting the error trap handler"
  trap on_failure EXIT

  log_info "Creating directories"
  create_directories

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Build and install 'go-fdo-client' binary"
  install_client

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Configuring services"
  configure_services

  # Start only manufacturer and owner (rendezvous excluded via services array override)
  log_info "Configure DNS and start services"
  start_services

  log_info "Wait for the services to be ready:"
  wait_for_services_ready

  log_info "Setting Rendezvous Info with RV BYPASS flag"
  # Override rv_info to point to owner with rv_bypass flag
  rv_info="[{\"dns\": \"${owner_dns}\", \"device_port\": \"${owner_port}\", \"protocol\": \"${owner_protocol}\", \"ip\": \"${owner_ip}\", \"owner_port\": \"${owner_port}\", \"rv_bypass\": true}]"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rv_info}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Adding Device CA certificate to owner"
  add_device_ca_cert "${owner_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Update rendezvous TO2 address"
  update_rvto2addr "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Running FIDO Device Onboard with RV bypass (without Rendezvous service)"
  run_fido_device_onboard "${guid}" --debug || log_error "Onboarding failed!"

  log_info "Unsetting the error trap handler"
  trap - EXIT
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
