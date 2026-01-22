#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

# FSIM fdo.wget specific configuration
fsim_wget_dir="${base_dir}/fsim/wget"

wget_httpd_service_name="wget_httpd"
wget_httpd_dir="${fsim_wget_dir}/httpd"
wget_httpd_log_file="${logs_dir}/http_server.log"
wget_httpd_dns="wget_httpd"
#shellcheck disable=SC2034
# needed for 'start_services' do not remove
wget_httpd_ip=127.0.0.1
wget_httpd_port=8888
wget_httpd_pid_file="${pid_dir}/http_server.pid"
wget_httpd_url="http://${wget_httpd_dns}:${wget_httpd_port}"
#shellcheck disable=SC2034
# needed for 'wait_for_services_ready' do not remove
wget_httpd_health_url="${wget_httpd_url}"

wget_file_name="file1"
wget_source_file="${wget_httpd_dir}/${wget_file_name}"
wget_source_url="${wget_httpd_url}/${wget_file_name}"

# create separate download directories for each device
wget_device1_download_dir="${fsim_wget_dir}/device1"
wget_device2_download_dir="${fsim_wget_dir}/device2"
wget_device1_download_file="${wget_device1_download_dir}/${wget_file_name}"
wget_device2_download_file="${wget_device2_download_dir}/${wget_file_name}"
declare -a wget_download_dirs=("${wget_device1_download_dir}" "${wget_device2_download_dir}")

start_service_wget_httpd() {
  # Start Python HTTP server in background
  cd "${wget_httpd_dir}"
  nohup python3 -m http.server ${wget_httpd_port} >"${wget_httpd_log_file}" 2>&1 &
  echo -n $! >"${wget_httpd_pid_file}"
  cd - >/dev/null
}

# Modified run_services function that adds wget support for owner service
start_service_owner() {
  run_go_fdo_server owner ${owner_service} owner ${owner_pid_file} ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --command-wget "${wget_source_url}"
}

run_test() {
  # Add the wget_httpd service defined above
  services+=("${wget_httpd_service_name}")

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  directories+=("${wget_httpd_dir}" "${wget_download_dirs[@]}")
  create_directories

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Build and install 'go-fdo-client' binary"
  install_client

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Configuring services"
  configure_services

  log_info "Start services"
  start_services

  log_info "Wait for the services to be ready:"
  wait_for_services_ready

  log_info "Prepare the wget test payload file on server side: '${wget_source_file}'"
  prepare_payload "${wget_source_file}"

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}" "${rendezvous_protocol}"

  log_info "Run Device Initialization for Device 1"
  guid=$(run_device_initialization)
  log_info "Device 1 initialized with GUID: ${guid}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Device 1 Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Running FIDO Device Onboard for Device 1 with FSIM fdo.wget"
  run_fido_device_onboard "${guid}" --debug --wget-dir "${wget_device1_download_dir}"

  log_info "Verify downloaded file ${wget_device1_download_file}"
  verify_equal_files "${wget_source_file}" "${wget_device1_download_file}"

  log_info "Device 1 Success!"

  log_info "Run Device Initialization For Device 2"
  guid=$(run_device_initialization)
  log_info "Device 2 initialized with GUID: ${guid}"

  log_info "Sending Device 2 Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Stop HTTP Server to Simulate Loss of WGET Service"
  stop_service "${wget_httpd_service_name}"

  log_info "Attempt WGET with missing HTTP server, verify FSIM error occurs"
  ! run_fido_device_onboard "${guid}" --debug --wget-dir "${wget_device2_download_dir}" ||
    log_error "Expected Device 2 onboard to fail!"

  log_info "Verifying the error was logged"
  # verify that the wget FSIM error is logged
  find_in_log "$(get_device_onboard_log_file_path "${guid}")" "error handling device service info .*fdo\.wget:error" ||
    log_error "The corresponding error was not logged"

  # Verify that Device 2 can successfully onboard once the HTTP server is available
  log_info "Restarting HTTP Server"
  start_service "${wget_httpd_service_name}"
  wait_for_service_ready "${wget_httpd_service_name}"

  log_info "Re-running FIDO Device Onboard with FSIM fdo.wget"
  run_fido_device_onboard "${guid}" --debug --wget-dir "${wget_device2_download_dir}"

  log_info "Verify downloaded file ${wget_device2_download_file}"
  verify_equal_files "${wget_source_file}" "${wget_device2_download_file}"

  log_info "Device 2 Success!"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
