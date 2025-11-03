#!/bin/bash
set -euox pipefail

# Import util functions
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

  log_info "Adding host entries for FDO services in host machine"
  echo -e "${manufacturer_ip} manufacturer\n${rendezvous_ip} rendezvous\n${owner_ip} owner" | sudo tee -a /etc/hosts > /dev/null

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Configuring services"
  configure_services

  log_info "Start services"
  start_services

  log_info "Wait for the services to be ready"
  wait_for_services_ready

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  log_info "Build bootc container from bootc base image"
  install_client

  log_info "Run Device Initialization"
  run_device_initialization

  log_info "Get device initialization voucher guid"
  guid=$(get_voucher_guid)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  sleep 60

  log_info "Running FIDO Device Onboard"
  run_fido_device_onboard || log_error "Onboarding failed!"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test; cleanup; }
