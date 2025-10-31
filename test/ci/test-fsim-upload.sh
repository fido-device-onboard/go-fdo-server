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

# Add the proper FSIM configuration via the owner server command line
setup_owner_cmdline() {
  owner_cmdline+=("--upload-directory=${owner_uploads_dir}")
  for file in "${upload_files[@]}"; do
    owner_cmdline+=("--command-upload=${file}")
  done
}

generate_upload_files() {
  cd ${device_uploads_dir}
  for device_file in "${upload_files[@]}"; do
    prepare_payload "${device_file}"
  done
  cd - >/dev/null
}

verify_uploads() {
  cd ${device_uploads_dir}
  for device_file in "${upload_files[@]}"; do
    owner_file="${owner_uploads_dir}/$(basename "${device_file}")"
    verify_equal_files "${owner_file}" "${device_file}"
  done
  cd - >/dev/null
}

# Public entrypoint used by CI
run_test() {

  echo "⭐ Creating directories"
  # Add uploads directories to be created
  directories+=("${device_uploads_dir}" "${owner_uploads_dir}")
  create_directories

  echo "⭐ Generating service certificates"
  generate_certs

  echo "⭐ Build and install 'go-fdo-client' binary"
  install_client

  echo "⭐ Build and install 'go-fdo-server' binary"
  install_server

  echo "⭐ Generating service configuration files"
  generate_service_configs

  echo "⭐ Set the owner server command line"
  setup_owner_cmdline

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
  run_to0 ${owner_url} "${guid}" >/dev/null

  echo "⭐ Prepare the upload payloads on client side: ${upload_files[*]}"
  generate_upload_files

  echo "⭐ Running FIDO Device Onboard with FSIM fdo.upload"
  run_fido_device_onboard --upload "/"

  echo "⭐ Verify uploaded files"
  verify_uploads

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
