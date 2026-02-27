#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-resale.sh"

run_test() {
  # Add the new owner service for wrong owner test
  services+=("${new_owner_service_name}")

  log_info "Setting the error trap handler"
  trap on_failure ERR

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
  add_device_ca_cert "${rendezvous_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Adding Device CA certificate to owner"
  add_device_ca_cert "${owner_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Get valid voucher from manufacturer"
  valid_ov="${base_dir}/valid.ov"
  get_ov_from_manufacturer "${manufacturer_url}" "${guid}" "${valid_ov}"

  log_info "Test 1: Valid voucher should be accepted"
  response=$(send_ov_to_owner "${owner_url}" "${valid_ov}")
  imported=$(echo "${response}" | jq -r '.imported')
  [[ "${imported}" == "1" ]] || log_error "Expected 1 voucher imported, got ${imported}"
  log_success "Valid voucher accepted"

  # NOTE: We use approximate offset-based corruption (not precise field-level corruption).
  # Precise field-level corruption is tested in unit tests (api/handlersTest/vouchers_test.go).
  # This approach is sufficient for E2E validation.
  # The new API returns HTTP 201 with imported=0 for invalid vouchers instead of HTTP 400.

  log_info "Corrupted voucher signature should be rejected"
  corrupted_ov="${base_dir}/corrupted_sig.ov"
  cp "${valid_ov}" "${corrupted_ov}"
  printf '\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF' | dd of="${corrupted_ov}" bs=1 seek=200 count=10 conv=notrunc 2>/dev/null
  response=$(send_ov_to_owner "${owner_url}" "${corrupted_ov}")
  imported=$(echo "${response}" | jq -r '.imported')
  [[ "${imported}" == "0" ]] || log_error "Expected 0 vouchers imported for corrupted voucher, got ${imported}"
  log_success "Corrupted voucher rejected"

  log_info "Test 3: Voucher with invalid cert chain hash should be rejected"
  invalid_hash_ov="${base_dir}/invalid_hash.ov"
  cp "${valid_ov}" "${invalid_hash_ov}"
  printf '\xAA\xBB\xCC\xDD\xEE\xFF' | dd of="${invalid_hash_ov}" bs=1 seek=120 count=6 conv=notrunc 2>/dev/null
  response=$(send_ov_to_owner "${owner_url}" "${invalid_hash_ov}")
  imported=$(echo "${response}" | jq -r '.imported')
  [[ "${imported}" == "0" ]] || log_error "Expected 0 vouchers imported for invalid hash, got ${imported}"
  log_success "Voucher with invalid cert chain hash rejected"

  log_info "Test 4: Voucher sent to wrong owner should be rejected"
  # Get the voucher and send to new owner
  wrong_owner_ov="${base_dir}/wrong_owner.ov"
  get_ov_from_manufacturer "${manufacturer_url}" "${guid}" "${wrong_owner_ov}"
  response=$(send_ov_to_owner "${new_owner_url}" "${wrong_owner_ov}")
  imported=$(echo "${response}" | jq -r '.imported')
  [[ "${imported}" == "0" ]] || log_error "Expected 0 vouchers imported for wrong owner, got ${imported}"
  log_success "New owner correctly rejected voucher (owner key doesn't match)"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
