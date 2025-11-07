#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../../ci/test-onboarding.sh"
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &>/dev/null && pwd)/utils.sh"

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

  echo "⭐ Start services"
  start_services

  echo "⭐ Wait for the services to be ready:"
  wait_for_services_ready

  echo "⭐ Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid ${device_credentials})
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Setting or updating Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Sleeping to allow TO0 to complete"
  sleep 20

  echo "⭐ Running FIDO Device Onboard "
  run_fido_device_onboard --debug

  echo "⭐ Cleaning up services configuration"
  cleanup_services_configuration

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
