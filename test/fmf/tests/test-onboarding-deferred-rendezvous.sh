#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-onboarding.sh"

run_test() {

  echo "⭐ Creating directories"
  create_directories

  echo "⭐ Generating service certificates"
  generate_certs

  echo "⭐ Build and install 'go-fdo-client' binary"
  install_client

  echo "⭐ Build and install 'go-fdo-server' binary"
  install_server

  echo "⭐ Configure services"
  configure_services

  echo "⭐ Setting hostnames"
  set_hostnames

  echo "⭐ Start services (manufacturer, owner) — rendezvous is intentionally delayed"
  start_service_manufacturer
  start_service_owner

  echo "⭐ Wait for manufacturer and owner to be ready"
  wait_for_service_ready manufacturer
  wait_for_service_ready owner

  echo "⭐ Setting or updating Rendezvous Info (RendezvousInfo) on manufacturer"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  # TODO: once we have an infinite loop in the client, we should just confirm that the onboarding process is just stuck until the rendezvous is started
  # This mimics the client reaching out to the rendezvous and getting a not found cause to0 is not done yet.
  echo "⭐ Attempting device onboarding before rendezvous is started (expect 'ERROR: TO1 failed')"
  pre_rv_log="$(mktemp)"
  run_go_fdo_client --blob "${device_credentials}" onboard --key ec256 --kex ECDH256 --insecure-tls=true --debug 2>&1 | tee "${pre_rv_log}" >/dev/null || true
  if grep -q "ERROR: TO1 failed" "${pre_rv_log}"; then
    echo "ℹ️ Observed expected failure reason before rendezvous is started: 'ERROR: TO1 failed'"
  else
    echo "❌ Expected 'ERROR: TO1 failed' before rendezvous is started"
    echo "ℹ️ Output:"
    tail -n 200 "${pre_rv_log}" || true
    rm -f "${pre_rv_log}"
    exit 1
  fi
  rm -f "${pre_rv_log}"

  echo "⭐ Now starting rendezvous"
  start_service_rendezvous
  wait_for_service_ready rendezvous

  echo "⭐ Sleeping 70 seconds to allow TO0 to complete"
  sleep 70

  echo "⭐ Running FIDO Device Onboard with retries until rendezvous/TO0 become available"
  # Retry onboarding up to 5 times, sleeping 10s between attempts
  max_attempts=5
  n=1
  success=0
  while [ ${n} -le ${max_attempts} ]; do
    echo "attempt-after-rv-${n}"
    if run_fido_device_onboard --debug; then
      success=1
      break
    fi
    n=$((n+1))
    sleep 10
  done

  if [ ${success} -ne 1 ]; then
    echo "❌ Device onboarding did not complete successfully after rendezvous start"
    exit 1
  fi

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test


