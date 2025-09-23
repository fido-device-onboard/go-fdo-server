#! /bin/bash

set -xeuo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"

# Download FSIM test configuration
download_test_dir="${base_dir}/tests/download-fsim"
download_files_dir="${download_test_dir}/files"
text_file_name="fdo-fsim-download.txt"
binary_file_name="app-binary"
owner_error_pattern="error opening.*for download FSIM"

# Cleanup function for download FSIM tests
cleanup_download_fsim() {
  echo "======================== Cleaning up download FSIM environment =========================="
  # Standard cleanup from test-makefile.sh
  cleanup

  # Remove test files created by our tests
  rm -rf "${download_test_dir}"
}

# Common function to run download FSIM test with specified files
run_fsim_download_test() {
  local test_name="$1"
  local file_type="$2"

  echo "======================== Testing FDO Download FSIM ($test_name) ========================"

  cleanup_download_fsim
  generate_certs
  install_client
  install_server

  # Generate test files after cleanup
  local test_files
  readarray -t test_files < <(generate_test_files "$file_type")
  local download_files=("${test_files[@]}")
  mkdir -p ${base_dir}

  # Makefile environment: run only required services with download FSIM
  setup_hostnames
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}

  # Build owner service command with download files
  local owner_cmd=(run_service owner ${owner_service} owner ${owner_log}
    --owner-key="${owner_key}"
    --device-ca-cert="${device_ca_crt}")

  for file in "${download_files[@]}"; do
    owner_cmd+=(--command-download="${file}")
  done

  "${owner_cmd[@]}"

  # Check if owner service failed due to non-existent download files
  sleep 2
  if [ -f "${owner_log}" ] && grep -q "${owner_error_pattern}" "${owner_log}"; then
    echo "✗ Owner service failed: non-existent download file"
    cat "${owner_log}"
    return 1
  fi

  # Wait for services to be ready
  wait_for_service "${manufacturer_service}"
  wait_for_service "${rendezvous_service}"
  wait_for_service "${owner_service}"

  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})

  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  local owner_onboard_log="${owner_onboard_log}"
  local download_dir="${base_dir}/downloads"

  # Create download directory and run onboarding with download enabled
  mkdir -p "${download_dir}"

  cd ${creds_dir}
  go-fdo-client --blob "${device_credentials}" --debug onboard --key ec256 --kex ECDH256 --download "${download_dir}" | tee "${owner_onboard_log}"
  cd -

  # Verify onboarding completed
  if grep -q 'FIDO Device Onboard Complete' "${owner_onboard_log}"; then
    echo "✓ FDO onboarding completed successfully"
  else
    echo "✗ FDO onboarding failed"
    return 1
  fi

  return 0
}

# Common function to generate test files for download FSIM testing
generate_test_files() {
  local file_type="$1"  # "single" or "multiple"
  local files_array=()

  mkdir -p "${download_files_dir}"

  # Always generate text file
  local text_file="${download_files_dir}/${text_file_name}"
  echo "Hello from FDO Download FSIM" > "${text_file}"
  echo "This is a test file for download functionality." >> "${text_file}"
  echo "Generated at runtime for testing." >> "${text_file}"
  files_array+=("${text_file}")

  # For multiple files, also generate binary file
  if [ "${file_type}" = "multiple" ]; then
    local binary_file="${download_files_dir}/${binary_file_name}"
    dd if=/dev/urandom of="${binary_file}" bs=1M count=2 2>/dev/null
    chmod +x "${binary_file}"
    echo "Created test file: ${binary_file} ($(stat -c%s "${binary_file}") bytes)" >&2
    files_array+=("${binary_file}")
  fi

  # Return the array of generated files
  printf '%s\n' "${files_array[@]}"
}

verify_download() {
  local src_file="$1"
  local dst_file="$2"

  [ -f "${dst_file}" ] || { echo "✗ FSIM download file not found: ${dst_file}"; return 1; }

  local src_sha dst_sha
  src_sha=$(sha256sum "${src_file}" | awk '{print $1}')
  dst_sha=$(sha256sum "${dst_file}" | awk '{print $1}')
  if [ "${src_sha}" != "${dst_sha}" ]; then
    echo "✗ Checksum mismatch: src=${src_sha} dst=${dst_sha}"
    return 1
  fi

  echo "✓ FSIM download verified at ${dst_file} (sha256=${dst_sha})"
}

test_single_file_download() {
  # Run common download test
  if ! run_fsim_download_test "Single File" "single"; then
    return 1
  fi

  # Verify download FSIM worked
  local download_dir="${base_dir}/downloads"
  local downloaded_file="${download_dir}/${text_file_name}"
  local source_file="${download_files_dir}/${text_file_name}"

  verify_download "${source_file}" "${downloaded_file}"
}

test_multiple_file_download() {
  # Run common download test
  if ! run_fsim_download_test "Multiple Files" "multiple"; then
    return 1
  fi

  # Verify multiple files were downloaded
  local download_dir="${base_dir}/downloads"
  local text_file="${download_files_dir}/${text_file_name}"
  local binary_file="${download_files_dir}/${binary_file_name}"

  local downloaded_text_file="${download_dir}/${text_file_name}"
  local downloaded_binary_file="${download_dir}/${binary_file_name}"

  # Verify both files using the simple verification function
  verify_download "${text_file}" "${downloaded_text_file}" && \
  verify_download "${binary_file}" "${downloaded_binary_file}" && \
  echo "✓ Multiple file download test passed"
}


test_fsim_download() {
  echo "======================== Running FDO Download FSIM Tests ========================"

  if ! test_single_file_download; then
    echo "======================== FAILED: Single file download test failed ========================"
    return 1
  fi

  if ! test_multiple_file_download; then
    echo "======================== FAILED: Multiple file download test failed ========================"
    return 1
  fi

  echo "======================== SUCCESS: All download FSIM tests passed! ========================"
  return 0
}

run_test() {
  cleanup
  test_fsim_download
}