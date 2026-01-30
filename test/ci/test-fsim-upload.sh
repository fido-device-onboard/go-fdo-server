#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-fsim-config.sh"

# FSIM fdo.upload specific configuration
fsim_upload_dir=${base_dir}/fsim/upload
owner_uploads_dir="${fsim_upload_dir}/owner"
device_uploads_dir="${fsim_upload_dir}/device"
device_uploads_subdir="${device_uploads_dir}/subdir"

# Device files are either relative to the client working dir or absolute.
# These are the source files:
device_files=("source-file1" "subdir/source-file2" "source-file3")

# Destination files on the owner. These are all relative to the $owner_uploads_dir/{GUID}.
# The last filename is taken from the source (see lack of dst: in configuration below)
owner_files=("dest-file1" "subdir/dest-file2" "source-file3")

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
      - fsim: "fdo.upload"
        params:
          dir: "${owner_uploads_dir}"
          files:
            - src: "${device_files[0]}"
              dst: "${owner_files[0]}"
            - src: "${device_files[1]}"
              dst: "${owner_files[1]}"
            - src: "${device_files[2]}"
EOF
}

generate_upload_files() {
  for device_file in "${device_files[@]}"; do
    if [ "${device_file:0:1}" != "/" ]; then
      # file is relative to the go-fdo-client working dir
      device_file="${credentials_dir:?}/${device_file}"
    fi
    prepare_payload "${device_file}"
  done
}

verify_uploads() {
  local device_guid=$1
  for ((i = 0; i < ${#device_files[@]}; i += 1)); do
    dst="${owner_uploads_dir}/${device_guid}/${owner_files[$i]}"
    src="${device_files[$i]}"
    if [ "${src:0:1}" != "/" ]; then
      # source is relative and was created in the go-fdo-client working dir
      src="${credentials_dir:?}/${src}"
    fi
    verify_equal_files "${src}" "${dst}"
  done
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
  directories+=("${device_uploads_subdir}" "${owner_uploads_dir}")
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

  log_info "Adding Device CA certificate to rendezvous"
  add_device_ca_cert "${rendezvous_url}" "${device_ca_crt}" | jq -r -M .

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Prepare the upload payloads on client side: ${device_files[*]}"
  generate_upload_files

  log_info "Running FIDO Device Onboard with FSIM fdo.upload"
  run_fido_device_onboard "${guid}"

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
