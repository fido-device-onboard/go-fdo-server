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

  echo "⭐ Creating directories"
  directories+=("${wget_httpd_dir}" "${wget_download_dirs[@]}")
  create_directories

  echo "⭐ Generating service certificates"
  generate_certs

  echo "⭐ Build and install 'go-fdo-client' binary"
  install_client

  echo "⭐ Build and install 'go-fdo-server' binary"
  install_server

  echo "⭐ Start services"
  start_services

  echo "⭐ Wait for the services to be ready:"
  wait_for_services_ready

  echo "⭐ Prepare the wget test payload file on server side: '${wget_source_file}'"
  prepare_payload "${wget_source_file}"

  echo "⭐ Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  echo "⭐ Run Device Initialization for Device 1"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device 1 initialized with GUID: ${guid}"

  echo "⭐ Sending Device 1 Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ Triggering TO0 on Owner server for Device 1 ${guid}"
  run_to0 ${owner_url} "${guid}" >/dev/null

  echo "⭐ Running FIDO Device Onboard for Device 1 with FSIM fdo.wget"
  run_fido_device_onboard --debug --wget-dir "${wget_device1_download_dir}"

  echo "⭐ Verify downloaded file ${wget_device1_download_file}"
  verify_equal_files "${wget_source_file}" "${wget_device1_download_file}"

  echo "⭐ Device 1 Success! ✅"

  echo "⭐ Run Device Initialization For Device 2"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device 2 initialized with GUID: ${guid}"

  echo "⭐ Sending Device 2 Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Triggering TO0 on Owner server for Device 2 ${guid}"
  run_to0 ${owner_url} "${guid}" >/dev/null

  echo "⭐ Stop HTTP Server to Simulate Loss of WGET Service"
  stop_service "${wget_httpd_service_name}"

  echo "⭐ Attempt WGET with missing HTTP server, verify FSIM error occurs"
  ! run_fido_device_onboard --debug --wget-dir "${wget_device2_download_dir}" || {
    echo "❌ Expected Device 2 onboard to fail!"
    return 1
  }

  # verify that the wget FSIM error is logged
  find_in_log_or_fail "$(get_device_onboard_log)" "error handling device service info .*fdo\.wget:error"

  # Verify that Device 2 can successfully onboard once the HTTP server is available
  echo "⭐ Restarting HTTP Server"
  start_service "${wget_httpd_service_name}"
  wait_for_service_ready "${wget_httpd_service_name}"

  echo "⭐ Re-running FIDO Device Onboard with FSIM fdo.wget"
  run_fido_device_onboard --debug --wget-dir "${wget_device2_download_dir}"

  echo "⭐ Verify downloaded file ${wget_device2_download_file}"
  verify_equal_files "${wget_source_file}" "${wget_device2_download_file}"

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
