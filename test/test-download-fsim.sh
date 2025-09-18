#! /bin/bash

set -xeuo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"

test_download_fsim() {
  echo "=============== Testing FDO Download FSIM ====================="

  cleanup
  generate_certs
  install_client
  install_server

  # Test file for download
  local test_file="$(pwd)/test/fsim-test-files/hello.txt"

  # Makefile environment: run only required services with download FSIM
  mkdir -p ${base_dir}
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}
  run_service owner ${owner_service} owner ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --command-download="${test_file}"

  # Setup hostnames and wait for services
  setup_hostnames
  wait_for_service "${manufacturer_service}"
  wait_for_service "${rendezvous_service}"
  wait_for_service "${owner_service}"

  echo "======================== Initialize device and run FSIM download test ==================="
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
  echo "Running onboarding with download FSIM..."
  go-fdo-client --blob "${device_credentials}" --debug onboard --key ec256 --kex ECDH256 --download "${download_dir}" | tee "${owner_onboard_log}"
  cd -

  # Verify onboarding completed
  if grep -q 'FIDO Device Onboard Complete' "${owner_onboard_log}"; then
    echo "✓ FDO onboarding completed successfully"
  else
    echo "✗ FDO onboarding failed"
    return 1
  fi

  # Verify download FSIM worked
  if [ -f "${download_dir}/hello.txt" ]; then
    echo "✓ File downloaded successfully: ${download_dir}/hello.txt"
    echo "✓ File size: $(stat -c%s "${download_dir}/hello.txt") bytes"

    chmod 644 "${download_dir}/hello.txt" 2>/dev/null || sudo chmod 644 "${download_dir}/hello.txt"

    if grep -q "Hello from FDO Download FSIM" "${download_dir}/hello.txt"; then
      echo "✓ File content verified!"
      echo "======================== SUCCESS: Download FSIM test passed! =========================="
      return 0
    else
      echo "✗ File content verification failed"
      cat "${download_dir}/hello.txt"
      return 1
    fi
  else
    echo "✗ File was not downloaded"
    echo "Downloaded files:"
    ls -la "${download_dir}/" || echo "Download directory not found"
    return 1
  fi
}