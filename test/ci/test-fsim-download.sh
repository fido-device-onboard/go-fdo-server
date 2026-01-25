#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-fsim-config.sh"

# FSIM fdo.download specific configuration
fsim_download_dir="${base_dir}/fsim/download"
owner_download_dir="${fsim_download_dir}/owner"
owner_download_subdir="${owner_download_dir}/subdir"
device_download_dir="${fsim_download_dir}/device"

# Owner files are all relative to the $owner_download_dir. These are the source files:
owner_files=("owner-file1" "owner-file2" "subdir/owner-file3")
# Destination files on the device. Either absolute, or relative to client working dir:
device_files=("${device_download_dir}/device-file1" "device-file2" "device-file3")

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
    fsims:
      - fsim: "fdo.download"
        params:
          dir: "${owner_download_dir}"
          files:
            - src: "${owner_files[0]}"
              dst: "${device_files[0]}"
            - src: "${owner_files[1]}"
              dst: "${device_files[1]}"
            - src: "${owner_files[2]}"
              dst: "${device_files[2]}"
EOF
}

generate_download_files() {
  cd "${owner_download_dir}"
  for owner_file in "${owner_files[@]}"; do
    prepare_payload "${owner_file}"
  done
  cd - >/dev/null
}

verify_downloads() {
  for (( i=0; i<${#owner_files[@]}; i+=1 )); do
    src="${owner_download_dir}/${owner_files[$i]}"
    dst="${device_files[$i]}"
    if [ "${dst:0:1}" != "/" ]; then
      # destination is relative and was written to the go-fdo-client working dir
      dst="${credentials_dir:?}/${dst}"
    fi
    verify_equal_files "${src}" "${dst}"
  done
}

# Public entrypoint used by CI
run_test() {

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  directories+=("$owner_download_subdir" "$device_download_dir")
  create_directories

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Build and install 'go-fdo-client' binary"
  install_client

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Configuring services"
  configure_services

  log_info "Generate the download payloads on owner side: ${owner_files[*]}"
  generate_download_files

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

  log_info "Running FIDO Device Onboard with FSIM fdo.download"
  run_fido_device_onboard "${guid}"

  log_info "Verify downloaded files"
  verify_downloads

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
