#! /usr/bin/env bash
#
# This test verifies that orphaned GUID-based upload directories are
# cleaned up when TO2 fails because a device file does not exist.
#
# Step 1: configure fdo.upload with a valid file and a non-existent file
# Step 2: first onboard attempt fails because the device cannot find
#         the non-existent file; the error triggers CleanupModules on
#         the Owner which removes the orphaned upload directory
# Step 3: stop the owner, reconfigure with only the valid file, restart
# Step 4: second onboard attempt succeeds
# Step 5: verify exactly one GUID directory remains
#
# This test builds both the client and server against kgiusti/go-fdo#issue-231
# which fixes error message delivery (go-fdo#232).

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-fsim-config.sh"

# go-fdo branch with the error message fix (go-fdo#232)
GO_FDO_FIX_REPO="https://github.com/kgiusti/go-fdo.git"
GO_FDO_FIX_BRANCH="issue-231"

# FSIM fdo.upload cleanup specific configuration
fsim_upload_dir="${base_dir}/fsim/upload"
owner_uploads_dir="${fsim_upload_dir}/owner"

# Valid device file to upload
device_file="upload-test-file"

# Non-existent device file that triggers the failure
missing_file="this-file-does-not-exist"

# Clone the patched go-fdo library with the error message fix.
setup_patched_go_fdo() {
  go_fdo_patch_dir="${base_dir}/go-fdo-patched"
  if [ ! -d "${go_fdo_patch_dir}" ]; then
    log_info "Cloning go-fdo fix from ${GO_FDO_FIX_REPO} (branch: ${GO_FDO_FIX_BRANCH})"
    git clone --depth 1 --branch "${GO_FDO_FIX_BRANCH}" "${GO_FDO_FIX_REPO}" "${go_fdo_patch_dir}"
  fi
}

# Override: build the client against the patched go-fdo library
install_client() {
  setup_patched_go_fdo
  local client_dir="${base_dir}/go-fdo-client-build"
  git clone --depth 1 https://github.com/fido-device-onboard/go-fdo-client.git "${client_dir}"
  pushd "${client_dir}" >/dev/null
  go mod edit -replace "github.com/fido-device-onboard/go-fdo=${go_fdo_patch_dir}"
  go mod tidy
  go install .
  popd >/dev/null
}

# Override: build the server against the patched go-fdo library
install_server() {
  setup_patched_go_fdo
  mkdir -p "${bin_dir}"
  go mod edit -replace "github.com/fido-device-onboard/go-fdo=${go_fdo_patch_dir}"
  go mod tidy
  make build && install -m 755 go-fdo-server "${bin_dir}" && rm -f go-fdo-server
  # Revert go.mod changes so we don't leave the repo dirty
  git checkout -- go.mod go.sum
}

# Owner config with both a valid file and a non-existent file
configure_service_owner_with_missing_file() {
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
            - src: "${device_file}"
            - src: "${missing_file}"
EOF
}

# Owner config with only the valid file (for the successful retry)
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
            - src: "${device_file}"
EOF
}

generate_upload_file() {
  prepare_payload "${credentials_dir}/${device_file}"
}

verify_upload_cleanup() {
  local guid_dirs
  guid_dirs=$(ls "${owner_uploads_dir}" | grep -c -e "^[a-f0-9]\{32\}$")

  if [ "${guid_dirs}" -ne 1 ]; then
    log_error "Expected exactly 1 GUID directory in ${owner_uploads_dir}, found ${guid_dirs}"
  fi

  local device_guid
  device_guid=$(ls "${owner_uploads_dir}" | grep -e "^[a-f0-9]\{32\}$")
  verify_equal_files "${credentials_dir}/${device_file}" "${owner_uploads_dir}/${device_guid}/${device_file}"
}

# Public entrypoint used by CI
run_test() {

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  directories+=("${owner_uploads_dir}")
  create_directories

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Build and install 'go-fdo-client' binary (with go-fdo error message fix)"
  install_client

  log_info "Build and install 'go-fdo-server' binary (with go-fdo error message fix)"
  install_server

  log_info "Configuring services (with missing upload file)"
  configure_service_manufacturer
  configure_service_rendezvous
  configure_service_owner_with_missing_file

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

  log_info "Prepare upload payload on client side: ${device_file}"
  generate_upload_file

  log_info "Running FIDO Device Onboard (expected to fail due to missing file)"
  # Use a short timeout so the client fails quickly instead of retrying
  # for the full 300s default. One retry cycle (~2 min) is enough to
  # create orphaned upload directories.
  client_timeout="30s"
  ! run_fido_device_onboard "${guid}" --debug ||
    log_error "Expected Device onboard to fail!"
  client_timeout="300s"

  log_info "Verifying the upload error was logged"
  find_in_log "$(get_device_onboard_log_file_path "${guid}")" "error uploading.*${missing_file}" ||
    log_error "Expected upload error for ${missing_file} not found in log"

  log_info "Stop owner, reconfigure with valid files only, restart"
  stop_service "owner"
  configure_service_owner
  start_service "owner"
  wait_for_service_ready "owner"

  log_info "Re-running FIDO Device Onboard (should succeed)"
  run_fido_device_onboard "${guid}" --debug

  log_info "Verify orphaned upload directory was cleaned up"
  verify_upload_cleanup

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
