#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

run_test() {

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

  log_info "Configure services"
  configure_services

  log_info "Setting hostnames"
  set_hostnames

  log_info "Start services (manufacturer, owner) — rendezvous is intentionally delayed"
  start_service_manufacturer
  start_service_owner

  log_info "Wait for manufacturer and owner to be ready"
  wait_for_service_ready manufacturer
  wait_for_service_ready owner

  log_info "Setting or updating Rendezvous Info (RendezvousInfo) on manufacturer"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Attempting device onboarding before rendezvous is started (expect 'ERROR: TO1 failed')"
  onboard_log="$(get_device_onboard_log_file_path "${guid}")"
  run_fido_device_onboard "${guid}" --debug &
  onboard_pid=$!

  log_info "Waiting for TO1 failure to appear in logs before starting rendezvous"
  found_to1_failure=false
  for _ in {1..30}; do
    if find_in_log "${onboard_log}" "ERROR: TO1 failed"; then
      found_to1_failure=true
      break
    fi
    sleep 1
  done

  if [ "${found_to1_failure}" != "true" ]; then
    log_error "Expected 'ERROR: TO1 failed' before rendezvous is started"
  fi

  log_info "Now starting rendezvous"
  start_service_rendezvous
  wait_for_service_ready rendezvous

  if ! wait "${onboard_pid}"; then
    log_error "Onboarding expected to succeed after rendezvous is started"
  fi
  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
