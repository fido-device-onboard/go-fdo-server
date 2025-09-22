#! /bin/bash

set -xeuo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"

uploads_dir=${base_dir}/uploads

trap 'fsim_teardown' EXIT

fsim_teardown() {
  echo "======================== Cleaning up FSIM upload environment =========================="
  # Delegate to standard cleanup from test-makefile.sh
  cleanup
}

setup_dirs() {
  echo "======================== Setting up directories =========================="
  mkdir -p "${uploads_dir}" "${creds_dir}"
  chmod -R 777 "${base_dir}" 2>/dev/null || true
}

# Start services with owner configured for upload FSIM
run_services_upload() {
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}
  run_service owner ${owner_service} owner ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --upload-directory="${uploads_dir}" \
    --command-upload uploaded.bin
}

start_services() {
  echo "======================== Starting services (local binaries) =========================="
  generate_certs
  install_client
  install_server
  setup_hostnames
  run_services_upload
  wait_for_service "${manufacturer_service}"
  wait_for_service "${rendezvous_service}"
  wait_for_service "${owner_service}"
  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port} || \
    update_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
}

prepare_upload_payload() {
  echo "======================== Creating binary upload payload in creds dir =========================="
  mkdir -p "${creds_dir}"
  dd if=/dev/urandom of="${creds_dir}/uploaded.bin" bs=1M count=2 2>/dev/null
  echo "Created test file: ${creds_dir}/uploaded.bin ($(stat -c%s "${creds_dir}/uploaded.bin") bytes)"
}

test_fsim_upload() {
  echo "======================== Running FDO onboarding with FSIM upload =========================="
  # Perform full onboarding steps and pass upload dir to client
  update_ips
  update_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log} --upload '/'
}

verify_upload() {
  echo "======================== Verifying FSIM upload (checksum only) =========================="
  local src_file="${creds_dir}/uploaded.bin"
  local dst_file="${uploads_dir}/uploaded.bin"

  [ -f "${dst_file}" ] || { echo "✗ FSIM upload file not found: ${dst_file}"; return 1; }

  local src_sha dst_sha
  src_sha=$(sha256sum "${src_file}" | awk '{print $1}')
  dst_sha=$(sha256sum "${dst_file}" | awk '{print $1}')
  if [ "${src_sha}" != "${dst_sha}" ]; then
    echo "✗ Checksum mismatch: src=${src_sha} dst=${dst_sha}"
    return 1
  fi

  echo "✓ FSIM upload verified at ${dst_file} (sha256=${dst_sha})"
}

# Public entrypoint used by CI
run_test() {
  echo "=============== Running FDO FSIM Upload Tests ====================="
  setup_dirs
  start_services
  prepare_upload_payload
  test_fsim_upload
  verify_upload
  echo "======================== SUCCESS: FSIM upload test passed! =========================="
}

# Allow running directly
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  run_test
fi 