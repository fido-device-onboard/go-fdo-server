#! /usr/bin/env bash
#
# This test verifies the fdo.wget FSIM
#
# Step 1: the test generates files to be served by the httpd server
# Step 2: configure the wget service module to download the files to
#         different locations on the device's filesystem
# Step 3: verify the device has downloaded files properly

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-fsim-config.sh"

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

# Three files to download from the http server:
wget_file1_name="file1"
wget_source_file1="${wget_httpd_dir}/${wget_file1_name}"
wget_source_url1="${wget_httpd_url}/${wget_file1_name}"
wget_file2_name="file2"
wget_source_file2="${wget_httpd_dir}/${wget_file2_name}"
wget_source_url2="${wget_httpd_url}/${wget_file2_name}"
wget_file3_name="file3"
wget_source_file3="${wget_httpd_dir}/${wget_file3_name}"
wget_source_url3="${wget_httpd_url}/${wget_file3_name}"

# download the files to three separate locations on the device:
# 1) relative to the go-fdo-client working directory with a new file name,
# 2) to an absolute path
# 3) to the working directory using the filename from the httpd host URL
wget_device_download_relative_file="relative_file1"
wget_device_download_absolute_dir="${fsim_wget_dir}/device"
wget_device_download_absolute_file="${wget_device_download_absolute_dir}/abs_file2"

configure_service_owner() {
  cat > "${owner_config_file}" <<EOF
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
    - fsim: "fdo.wget"
      params:
        files:
          - url: "${wget_source_url1}"
            dst: "${wget_device_download_relative_file}"
          - url: "${wget_source_url2}"
            dst: "${wget_device_download_absolute_file}"
          - url: "${wget_source_url3}"
EOF
}

start_service_wget_httpd() {
  # Start Python HTTP server in background
  cd "${wget_httpd_dir}"
  nohup python3 -m http.server ${wget_httpd_port} >"${wget_httpd_log_file}" 2>&1 &
  echo -n $! >"${wget_httpd_pid_file}"
  cd - >/dev/null
}

run_test() {
  # Add the wget_httpd service defined above
  services+=("${wget_httpd_service_name}")

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  directories+=("${wget_httpd_dir}" "${wget_device_download_absolute_dir}")
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

  log_info "Prepare the wget test payload file on server side: '${wget_source_file1}', '${wget_source_file2}'"
  prepare_payload "${wget_source_file1}"
  prepare_payload "${wget_source_file2}"
  prepare_payload "${wget_source_file3}"

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rv_info}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Device Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Stop HTTP Server to Simulate Loss of WGET Service"
  stop_service "${wget_httpd_service_name}"

  log_info "Attempt WGET with missing HTTP server, verify FSIM error occurs"
  ! run_fido_device_onboard "${guid}" --debug ||
    log_error "Expected Device onboard to fail!"

  log_info "Verifying the error was logged"
  # verify that the wget FSIM error is logged
  find_in_log "$(get_device_onboard_log_file_path "${guid}")" "error handling device service info .*fdo\.wget:error" ||
    log_error "The corresponding error was not logged"

  # Verify that Device can successfully onboard once the HTTP server is available
  log_info "Restarting HTTP Server"
  start_service "${wget_httpd_service_name}"
  wait_for_service_ready "${wget_httpd_service_name}"

  log_info "Re-running FIDO Device Onboard with FSIM fdo.wget"
  run_fido_device_onboard "${guid}" --debug

  # Note: go-fdo-client onboard executes in the ${credentials_dir} directory, expect
  # to find the relative pathnamed files there:
  log_info "Verify downloaded file ${credentials_dir}/${wget_device_download_relative_file}"
  verify_equal_files "${wget_source_file1}" "${credentials_dir}/${wget_device_download_relative_file}"

  log_info "Verify downloaded file ${wget_device_download_absolute_file}"
  verify_equal_files "${wget_source_file2}" "${wget_device_download_absolute_file}"

  log_info "Verify downloaded file ${credentials_dir}/${wget_file3_name}"
  verify_equal_files "${wget_source_file3}" "${credentials_dir}/${wget_file3_name}"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
