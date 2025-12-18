#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

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

  log_info "Adding the certs to the rendezvous server"
  add_device_ca_cert ${rendezvous_url} ${device_ca_crt}
  add_device_ca_cert ${rendezvous_url} ${owner_crt}
  add_device_ca_cert ${rendezvous_url} ${manufacturer_crt}

  add_device_ca_cert ${rendezvous_url} ${device_ca_crt}
  add_device_ca_cert ${rendezvous_url} ${owner_crt}
  add_device_ca_cert ${rendezvous_url} ${manufacturer_crt}

  log_info "Retrieving the rendezvous device ca certs"
  get_device_ca_certs ${rendezvous_url}

  log_info "Unsetting the error trap handler"
  trap - EXIT
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
