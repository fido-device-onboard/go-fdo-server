#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

new_owner_service_name=new_owner
new_owner_dns=new_owner
# needed for 'start_services' do not remove
#shellcheck disable=SC2034
new_owner_ip=127.0.0.1
new_owner_port=8045
new_owner_pid_file="${pid_dir}/new_owner.pid"
new_owner_log="${logs_dir}/${new_owner_dns}.log"
# key crt pub and subj variables are required to generate certificates
new_owner_key="${certs_dir}/new_owner.key"
#shellcheck disable=SC2034
new_owner_crt="${new_owner_key/\.key/.crt}"
new_owner_pub="${new_owner_key/\.key/.pub}"
#shellcheck disable=SC2034
new_owner_subj="/C=US/O=FDO/CN=New Owner"
new_owner_service="${new_owner_dns}:${new_owner_port}"
new_owner_protocol="http"
new_owner_url="${new_owner_protocol}://${new_owner_service}"
# needed for 'wait_for_services_ready' do not remove
#shellcheck disable=SC2034
new_owner_health_url="${new_owner_url}/health"
# The file where the new owner voucher will be saved after the resale protocol has been run
new_owner_ov="${base_dir}/new_owner.ov"

#shellcheck disable=SC2034
new_owner_https_subj="/C=US/O=FDO/CN=new_owner"
new_owner_https_key="${certs_dir}/new_owner-http.key"
new_owner_https_crt="${certs_dir}/new_owner-http.crt"

start_service_new_owner() {
  local extra_opts=()
  if [ "${new_owner_protocol}" = "https" ]; then
    extra_opts+=(--http-cert "${new_owner_https_crt}" --http-key "${new_owner_https_key}" --to0-insecure-tls)
  fi
  run_go_fdo_server owner ${new_owner_service} new_owner ${new_owner_pid_file} ${new_owner_log} \
    --owner-key="${new_owner_key}" \
    --device-ca-cert="${device_ca_crt}"
    "${extra_opts[@]}"
}

run_test() {
  # Add the new owner service defined above
  services+=("${new_owner_service_name}")

  echo "⭐ Setting the trap handler in case of error"
  trap on_failure ERR

  echo "⭐ Environment variables"
  printenv|sort

  echo "⭐ Creating directories"
  create_directories

  echo "⭐ Generating service certificates"
  generate_service_certs

  echo "⭐ Build and install the 'go-fdo-client' binary"
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

  guid=$(get_device_guid "${device_credentials}")
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Extracting the public key from the New Owner cert"
  extract_pubkey_from_cert ${new_owner_crt} ${new_owner_pub}

  echo "⭐ Trigger the Resell protocol on the current owner"
  resell "${owner_url}" "${guid}" "${new_owner_pub}" "${new_owner_ov}"

  echo "⭐ Setting or updating the New Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${new_owner_url}" "${new_owner_service_name}" "${new_owner_dns}" "${new_owner_port}" "${new_owner_protocol}"

  echo "⭐ Sending the Ownership Voucher to the New Owner"
  send_ov_to_owner "${new_owner_url}" "${new_owner_ov}"

  echo "⭐ Sleeping to allow TO0 to complete"
  sleep 20

  echo "⭐ Running FIDO Device Onboard"
  run_fido_device_onboard --debug

  echo "⭐ Unsetting the trap handler in case of error"
  trap - ERR

  echo "✅ Test PASS!"
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test && cleanup; }
