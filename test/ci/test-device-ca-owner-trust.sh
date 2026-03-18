#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

certs_file="${logs_dir}/certs.json"

run_test() {

  log_info "Setting the error trap handler"
  trap on_failure EXIT

  log_info "Environment variables"
  show_env

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

  log_info "Configure DNS and start services"
  start_services

  log_info "Wait for the services to be ready:"
  wait_for_services_ready

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rv_info}"

  log_info "Adding Device CA certificate to rendezvous"
  add_device_ca_cert ${rendezvous_url} ${device_ca_crt} | jq -r -M .

  log_info "Update rendezvous TO2 address"
  update_rvto2addr "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Sending Ownership Voucher to the Owner"
  result=$(send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}" | jq -r -M '.')
  echo "${result}"
  imported=$(echo "${result}" | jq -r -M '.imported' || log_error "Cannot determine how many ownership vouchers were imported")
  [ "${imported}" = "0" ] || log_error "Voucher should not be accepted!"

  echo ${result} | grep "${guid}" | grep "cryptographic verification failed: x509: certificate signed by unknown authority" || log_error "The owner didn't reject the ownership voucher!"

  log_info "Adding Device CA certificate to owner"
  add_device_ca_cert ${owner_url} ${device_ca_crt} | jq -r -M .

  log_info "List the owner Device CA certificates"
  fingerprint=$(get_device_ca_certs ${owner_url} | jq -r -M '.certs[0].fingerprint')

  log_info "Deleting certificate with fingerprint '${fingerprint}'"
  delete_device_ca_cert ${owner_url} ${fingerprint}

  log_info "Adding Device CA certificate again to owner"
  add_device_ca_cert ${owner_url} ${device_ca_crt} | jq -r -M .

  log_info "Sending Ownership Voucher to the Owner"
  result=$(send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}")
  [ "$(echo "${result}" | jq -r -M '.imported')" = "1" ] || log_error "Voucher should be accepted!"

  log_info "Running FIDO Device Onboard"
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
