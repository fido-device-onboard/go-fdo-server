#!/bin/bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/utils.sh"

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

wget_test_file_name="file1"
wget_test_file="${wget_httpd_dir}/${wget_test_file_name}"
wget_test_url="${wget_httpd_url}/${wget_test_file_name}"

wget_device_download_dir="${fsim_wget_dir}/download"
wget_device_test_file="${wget_device_download_dir}/${wget_test_file_name}"


start_service_wget_httpd() {
  # Start Python HTTP server in background
  cd "${wget_httpd_dir}"
  nohup python3 -m http.server ${wget_httpd_port} > "${wget_httpd_log_file}" 2>&1 &
  echo -n $! > "${wget_httpd_pid_file}"
  cd - > /dev/null
}

# Modified run_services function that adds wget support for owner service
start_service_owner() {
  run_go_fdo_server owner ${owner_service} owner ${owner_pid_file} ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --command-wget "${wget_test_url}"
}

run_test () {
  # Add the wget_httpd service defined above
  services+=("${wget_httpd_service_name}")

  echo "⭐ Creating directories"
  directories+=("${wget_httpd_dir}" "${wget_device_download_dir}")
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

  echo "⭐ Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ Triggering TO0 on Owner server"
  run_to0 ${owner_url} "${guid}" > /dev/null

  echo "⭐ Prepare the wget payload on server side: '${wget_test_file}'"
  prepare_payload "${wget_test_file}"

  echo "⭐ Running FIDO Device Onboard with FSIM fdo.wget"
  run_fido_device_onboard --debug --wget-dir "${wget_device_download_dir}"

  echo "⭐ Verify downloaded file: server: ${wget_test_file} device: ${wget_device_test_file}"
  verify_equal_files "${wget_test_file}" "${wget_device_test_file}"

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
