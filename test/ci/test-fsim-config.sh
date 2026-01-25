#! /usr/bin/env bash
#
# This test verifies that a sequence of FSIMs are performed in the proper order.
#
# Step 1: download a bash script to the device
# Step 2: have the device run the bash script with command line
#         arguments, the script generates output
# Step 3: upload the generated output
#
# The test passes if the output is generated as expected

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-onboarding-config.sh"

# All relative filepaths on the device are expected to be relative to
# the current directory of the process running go-fdo-client
# onboarding (see run_go_fdo_client function in utils.sh)
device_output_subdir="output"
device_output_filename="script-out.txt"

owner_download_dir="${base_dir}/fsim/download"
owner_upload_dir="${base_dir}/fsim/upload"

test_script_name="test-script.sh"
test_file_content="Hello from the FDO client!"

# https://github.com/fido-device-onboard/go-fdo/issues/210
# When uploading or downloading files to go-fdo library creates a
# temporary file then uses os.Rename() to "move" the file to the
# proper destination. This fails on fedora/rhel because the default
# temporary directory /tmp is a tmpfs mount and os.Rename cannot
# "move" files across filesystems. This really needs to be fixed in
# the go-fdo library.
tmp_dir="${base_dir}/tmp"
directories+=("${tmp_dir}")
export TMPDIR="${tmp_dir}"

# This creates the test script in the download directory of the owner server
# Note that the test downloads this script to the device and gives it
# a different filename on the device to ensure the device correctly
# executes its local copy of the script
generate_test_script() {
  cat > "${owner_download_dir}/${test_script_name}" <<EOF
#!/bin/bash
set -ueo pipefail
outdir="\${1:?}"
outfile="\${2:?}"
#shellcheck disable=SC2154
mkdir -p "\${outdir}"
cd "\${outdir}"
#shellcheck disable=SC2154
echo "${test_file_content}" > "\${outfile}"
EOF
}

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
            - src: "${test_script_name}"
              dst: "device-script.sh"
      - fsim: "fdo.command"
        params:
          cmd: "chmod"
          args:
            - "+rx"
            - "device-script.sh"
      - fsim: "fdo.command"
        params:
          return_stdout: true
          return_stderr: true
          cmd: "./device-script.sh"
          args:
            - "${device_output_subdir}"
            - "${device_output_filename}"
      - fsim: "fdo.upload"
        params:
          dir: "${owner_upload_dir}"
          files:
            - src: "${device_output_subdir}/${device_output_filename}"
EOF
}

# Public entrypoint used by CI
run_test() {

  log_info "Setting the error trap handler"
  trap on_failure ERR

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  directories+=("${owner_download_dir}" "${owner_upload_dir}")
  create_directories

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Build and install 'go-fdo-client' binary"
  install_client

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Configuring services"
  configure_services

  log_info "Generate test script"
  generate_test_script

  log_info "Configure DNS and start services"
  start_services

  log_info "Wait for the services to be ready:"
  wait_for_services_ready

  log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}" "${rendezvous_protocol}"

  log_info "Run Device Initialization"
  guid=$(run_device_initialization)
  log_info "Device initialized with GUID: ${guid}"

  log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "${owner_protocol}"

  log_info "Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  log_info "Running FIDO Device Onboard with FSIM fdo.command"
  run_fido_device_onboard "${guid}" --debug

  log_info "Verifying the results of the onboarding FSIM operations"
  # Currently there is no way to get the replacement GUID for the device,
  # which is what the upload uses for the destination directory name. For now
  # read the directory and assume the result is correct
  local guid=$(ls "${owner_upload_dir}" | grep -e "^[a-f0-9]\{32\}$")
  expected_file="${owner_upload_dir}/${guid}/${device_output_filename}"
  if [ ! -e "${expected_file}" ]; then
    log_error "Expected file ${expected_file} not present"
  fi
  expected_sha=$(echo "${test_file_content}" | sha256sum | awk '{print $1}')
  actual_sha=$(sha256sum "${expected_file}" | awk '{print $1}')
  if [ "${expected_sha}" != "${actual_sha}" ]; then
    log_error "File checksum mismatch: expected=${expected_sha} actual=${actual_sha}"
  fi

  log_info "Unsetting the error trap handler"
  trap - ERR
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test; cleanup; }
