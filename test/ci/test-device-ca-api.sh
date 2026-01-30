#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

certs_file="${logs_dir}/certs.json"
multiple_certs_file="${logs_dir}/device_ca_certs.pem"

get_cert_array_length() {
  jq -r -M '.certs| length' "${certs_file}"
}

get_cert_fingerprint() {
  local i=$1
  jq -r -M ".certs[$i].fingerprint" "${certs_file}"
}

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

  expected="1"
  log_info "Uploading ${expected} certs to the rendezvous server"
  add_device_ca_cert ${rendezvous_url} ${device_ca_crt} | jq -r -M .
  log_info "Refresh the device CA certs from API"
  get_device_ca_certs ${rendezvous_url} | jq -r -M . | tee "${certs_file}"
  actual="$(get_cert_array_length)"
  log_info "Checking certs array is correct after the upload"
  [ "${actual}" = "${expected}" ] || log_error "Unexpected cert array size, expected: '${expected}' actual: '${actual}'"

  expected="1"
  log_info "Uploading the same cert to the rendezvous server"
  add_device_ca_cert ${rendezvous_url} ${device_ca_crt} | jq -r -M .
  log_info "Refresh the device CA certs from API"
  get_device_ca_certs ${rendezvous_url} | jq -r -M . | tee "${certs_file}"
  actual="$(get_cert_array_length)"
  log_info "Checking certs array is correct after the upload of existing cert"
  [ "${actual}" = "${expected}" ] || log_error "Unexpected cert array size, expected: '${expected}' actual: '${actual}'"

  expected="3"
  log_info "Uploading the same cert + 2 additional certs from the same file"
  cat "${device_ca_crt}" "${owner_crt}" "${manufacturer_crt}" >>"${multiple_certs_file}"
  add_device_ca_cert ${rendezvous_url} ${multiple_certs_file} | jq -r -M .
  log_info "Refresh the device CA certs from API"
  get_device_ca_certs ${rendezvous_url} | jq -r -M . | tee "${certs_file}"
  actual="$(get_cert_array_length)"
  log_info "Checking certs array is correct after the upload of multiple certs"
  [ "${actual}" = "${expected}" ] || log_error "Unexpected cert array size, expected: '${expected}' actual: '${actual}'"

  log_info "Deleting the first device ca certificate"
  fingerprint=$(get_cert_fingerprint 0)
  delete_device_ca_cert ${rendezvous_url} ${fingerprint} | jq -r -M . | tee "${certs_file}"

  log_info "Refresh the device CA certs from API"
  get_device_ca_certs ${rendezvous_url} | jq -r -M . | tee "${certs_file}"
  expected="2"
  actual="$(get_cert_array_length)"
  log_info "Checking certs array is correct after removing one"
  [ "${actual}" = "${expected}" ] || log_error "Unexpected cert array size, expected: '${expected}' actual: '${actual}'"

  log_info "Unsetting the error trap handler"
  trap - EXIT
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
