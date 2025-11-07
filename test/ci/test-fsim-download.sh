#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

# FSIM fdo.download specific configuration
fsim_download_dir="${base_dir}/fsim/download"
owner_download_dir="${fsim_download_dir}/owner"
device_download_dir="${fsim_download_dir}/device"

# downloads using relative subdir paths doesn't work
#download_files=("relative1" "subdir1/relative2" "subdir1/subdir2/relative3" "${owner_download_dir}/absolute")
download_files=("file1" "${owner_download_dir}/file2" "${owner_download_dir}/subdir1/file3")

# Overwrite the owner service start function to configure download FSIM
start_service_owner() {
  download_commands=()
  for file in "${download_files[@]}"; do
    download_commands+=("--command-download=${file}")
  done
  cd ${owner_download_dir}
  run_go_fdo_server owner ${owner_service} owner ${owner_pid_file} ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    "${download_commands[@]}"
  cd - >/dev/null
}

generate_download_files() {
  cd ${owner_download_dir}
  for owner_file in "${download_files[@]}"; do
    prepare_payload "${owner_file}"
  done
  cd - >/dev/null
}

verify_downloads() {
  cd ${owner_download_dir}
  for owner_file in "${download_files[@]}"; do
    device_file="${device_download_dir}/$(basename "${owner_file}")"
    verify_equal_files "${device_file}" "${owner_file}"
  done
  cd - >/dev/null
}

# Public entrypoint used by CI
run_test() {

  echo "⭐ Setting the trap handler in case of error"
  trap on_failure ERR

  echo "⭐ Environment variables"
  printenv|sort

  echo "⭐ Creating directories"
  directories+=("$owner_download_dir" "$device_download_dir")
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

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Sleeping to allow TO0 to complete"
  sleep 20

  echo "⭐ Generate the download payloads on owner side: ${download_files[*]}"
  generate_download_files

  echo "⭐ Running FIDO Device Onboard with FSIM fdo.download"
  run_fido_device_onboard --download "${device_download_dir}"

  echo "⭐ Verify downloaded files"
  verify_downloads

  echo "⭐ Unsetting the trap handler in case of error"
  trap - ERR

  echo "✅ Test PASS!"
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
