#! /usr/bin/env bash
# RV bypass test: Device skips TO1 by getting Owner address directly from voucher (TO0 not needed)

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

verify_rv_bypass_behavior() {
  local guid="$1"
  local device_log="${logs_dir}/onboarding-device-${guid}.log"

  echo "  🔍 Checking device onboard log for bypass indicators..."

  # Device onboarding completed successfully
  if grep -q "FIDO Device Onboard Complete" "${device_log}" || grep -q "Credential Reuse Protocol" "${device_log}"; then
    echo "  ✅ Device onboarding completed"
  else
    echo "  ❌ Device onboarding did not complete"
    cat "${device_log}"
    return 1
  fi

  # Rendezvous service was NOT contacted (device skips TO1, no TO0 needed)
  if [ -f "${rendezvous_log}" ]; then
    echo "  ❌ Rendezvous log exists but should not with RV bypass!"
    return 1
  fi
  echo "  ✅ Rendezvous service was not used (device skipped TO1)"

  # Owner received TO2 directly (msg/60)
  find_in_log_or_fail "${owner_log}" 'msg/60'
  echo "  ✅ Owner received TO2 protocol messages (direct connection)"

  # Owner completed TO2 (msg/70)
  find_in_log_or_fail "${owner_log}" 'msg/70'
  echo "  ✅ Owner completed TO2 protocol"

  echo "  🎉 RV bypass behavior verified successfully!"
}

run_test() {

  echo "⭐ Creating directories"
  create_directories

  echo "⭐ Cleaning up any old log files"
  rm -f "${logs_dir}"/*.log

  echo "⭐ Generating service certificates"
  generate_service_certs

  echo "⭐ Build and install 'go-fdo-client' binary"
  install_client

  echo "⭐ Build and install 'go-fdo-server' binary"
  install_server

  echo "⭐ Adding hostnames to /etc/hosts"
  set_hostnames

  echo "⭐ Start Manufacturing and Owner services (NO Rendezvous with bypass!)"
  start_service "${manufacturer_service_name}"
  start_service "${owner_service_name}"

  echo "⭐ Wait for the services to be ready:"
  wait_for_service_ready "${manufacturer_service_name}"
  wait_for_service_ready "${owner_service_name}"

  echo "⭐ Setting Rendezvous Info with RV BYPASS flag"
  set_or_update_rendezvous_info "${manufacturer_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}" "http" "true"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid)
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ SKIPPING TO0 (no need - device will skip TO1 via RV bypass)"

  echo "⭐ Running FIDO Device Onboard with RV bypass"
  run_fido_device_onboard --debug

  echo "⭐ Saving container logs (no-op for binary tests)"
  save_logs

  echo "⭐ Verifying RV bypass behavior in logs"
  verify_rv_bypass_behavior "${guid}"

  echo "⭐ Success! ✅"
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test; cleanup; }
