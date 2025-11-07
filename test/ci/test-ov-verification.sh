#! /bin/bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-resale.sh"

run_test () {
  # Add the new owner service for wrong owner test
  services+=("${new_owner_service_name}")

  echo "⭐ Setting the trap handler in case of error"
  trap on_failure ERR

  echo "⭐ Environment variables"
  printenv|sort

  echo "⭐ Creating directories"
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
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}" "${rendezvous_protocol}"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Get valid voucher from manufacturer"
  valid_ov="${base_dir}/valid.ov"
  get_ov_from_manufacturer "${manufacturer_url}" "${guid}" "${valid_ov}"

  echo "  ⭐ Test 1: Valid voucher should be accepted"
  send_ov_to_owner "${owner_url}" "${valid_ov}" 2>&1 || { echo "  ❌ This test was supposed to succeed"; return 1; }
  echo "  ✅ Valid voucher accepted"

  # NOTE: We use approximate offset-based corruption (not precise field-level corruption).
  # Precise field-level corruption is tested in unit tests (api/handlersTest/vouchers_test.go).
  # This approach is sufficient for E2E validation.

  echo "  ⭐ Test 2: Corrupted voucher signature should be rejected"
  corrupted_ov="${base_dir}/corrupted_sig.ov"
  cp "${valid_ov}" "${corrupted_ov}"
  printf '\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF' | dd of="${corrupted_ov}" bs=1 seek=200 count=10 conv=notrunc 2>/dev/null
  ! send_ov_to_owner "${owner_url}" "${corrupted_ov}" 2>&1 || { echo "  ❌ This test was supposed to fail"; return 1; }
  echo "  ✅ Corrupted voucher rejected"

  echo "  ⭐ Test 3: Voucher with invalid cert chain hash should be rejected"
  invalid_hash_ov="${base_dir}/invalid_hash.ov"
  cp "${valid_ov}" "${invalid_hash_ov}"
  printf '\xAA\xBB\xCC\xDD\xEE\xFF' | dd of="${invalid_hash_ov}" bs=1 seek=120 count=6 conv=notrunc 2>/dev/null
  ! send_ov_to_owner "${owner_url}" "${invalid_hash_ov}" 2>&1 || { echo "  ❌ This test was supposed to fail"; return 1; }
  echo "  ✅ Voucher with invalid cert chain hash rejected"

  echo "  ⭐ Test 4: Voucher sent to wrong owner should be rejected"
  ! send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${new_owner_url}" 2>&1 || { echo "  ❌ This test was supposed to fail"; return 1; }
  echo "  ✅ New owner correctly rejected voucher (owner key doesn't match)"

  echo "⭐ Unsetting the trap handler in case of error"
  trap - ERR

  echo "✅ Test PASS!"
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
