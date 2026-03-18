#! /usr/bin/env bash
#
# This test verifies the fdo.command FSIM
#
# Step 1: run "touch" on the device to create an empty test file
# Step 2: run a command that renames the test file
#
# Verify that the file exists in the working directory of the
# go-fdo-client and has the expected name.

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-fsim-config.sh"

configure_service_owner() {
  cat >"${owner_config_file}" <<EOF
log:
  level: "debug"
db:
  type: "sqlite"
  dsn: "file:${base_dir}/owner.db"
http:
  ip: "${owner_dns}"
  port: ${owner_port}
device_ca:
  cert: "${device_ca_crt}"
owner:
  key: "${owner_key}"
  to0_insecure_tls: true
  service_info:
    fsims:
      - fsim: "fdo.command"
        params:
          may_fail: false
          return_stdout: true
          cmd: "touch"
          args: ["firstCommand.txt"]
      - fsim: "fdo.command"
        params:
          may_fail: false
          return_stdout: false
          cmd: "bash"
          args:
            - "-c"
            - |
              set -xeuo pipefail
              if [ -a firstCommand.txt ]; then
                  mv firstCommand.txt commandSuccess.txt
              fi
EOF
}

# Public entrypoint used by CI
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

  log_info "Configuring services"
  configure_services

  log_info "Configure DNS and start services"
  start_services

  log_info "Wait for the services to be ready:"
  wait_for_services_ready

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rv_info}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Update rendezvous TO2 address"
  update_rvto2addr "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Adding Device CA certificate to rendezvous"
  add_device_ca_cert "${rendezvous_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Adding Device CA certificate to owner"
  add_device_ca_cert "${owner_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Running FIDO Device Onboard with FSIM fdo.command"
  run_fido_device_onboard "${guid}" --debug

  log_info "Verifying the results of the fdo.command operation"
  [ -a "${credentials_dir}/commandSuccess.txt" ] ||
    log_error "Expected file ${credentials_dir}/commandSuccess.txt not present"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
