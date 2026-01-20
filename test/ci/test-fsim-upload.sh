#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

# FSIM fdo.upload specific configuration
fsim_upload_dir=${base_dir}/fsim/upload
owner_uploads_dir="${fsim_upload_dir}/owner"
device_uploads_dir="${credentials_dir}"

# Uploads using absolute paths doesn't work
#upload_files=("relative1" "${device_uploads_dir}/absolute1" "${device_uploads_dir}/subdir1/absolute2")
upload_files=("file1" "subdir1/file2" "subdir1/subdir2/file3")

# Overwrite the owner service start function to configure upload FSIM
start_service_owner() {
  upload_commands=()
  for file in "${upload_files[@]}"; do
    upload_commands+=("--command-upload=${file}")
  done

  run_go_fdo_server owner ${owner_service} owner ${owner_pid_file} ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --upload-directory="${owner_uploads_dir}" \
    "${upload_commands[@]}"
}

generate_upload_files() {
  cd ${device_uploads_dir}
  for device_file in "${upload_files[@]}"; do
    prepare_payload "${device_file}"
  done
  cd - >/dev/null
}

verify_uploads() {
  local device_guid=$1
  cd ${device_uploads_dir}
  for device_file in "${upload_files[@]}"; do
    owner_file="${owner_uploads_dir}/${device_guid}/$(basename "${device_file}")"
    verify_equal_files "${owner_file}" "${device_file}"
  done
  cd - >/dev/null
}

get_device_guid() {
  local owner_url=$1
  local guid=$2
  local device_guid=$(curl -s "${owner_url}/api/v1/owner/devices?old_guid=${guid}" | jq -r '.[0].guid')
  echo "${device_guid}"
}

# Public entrypoint used by CI
run_test() {

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  # Add uploads directories to be created
  directories+=("${device_uploads_dir}" "${owner_uploads_dir}")
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

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Prepare the upload payloads on client side: ${upload_files[*]}"
  generate_upload_files

  log_info "Running FIDO Device Onboard with FSIM fdo.upload"
  run_fido_device_onboard "${guid}" --upload "/"

  device_guid=$(get_device_guid "${owner_url}" "${guid}")
  log_info "Device GUID after onboarding: ${device_guid}"

  log_info "Verify uploaded files"
  verify_uploads "${device_guid}"

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
