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
new_owner_url="http://${new_owner_service}"
# needed for 'wait_for_services_ready' do not remove
#shellcheck disable=SC2034
new_owner_health_url="${new_owner_url}/health"
# The file where the new owner voucher will be saved after the resale protocol has been run
new_owner_ov="${base_dir}/new_owner.ov"
new_owner_config_file="${configs_dir}/new_owner.yaml"
declare -a new_owner_cmdline=("--debug" "--config=${new_owner_config_file}")

generate_new_owner_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "sqlite"
  dsn: "file:${base_dir}/new_owner.db"
http:
  ip: "${new_owner_dns}"
  port: ${new_owner_port}
owner:
  device_ca_cert: "${device_ca_crt}"
  key: "${new_owner_key}"
  to0_insecure_tls: true
EOF
}

start_service_new_owner() {
  run_go_fdo_server owner ${new_owner_pid_file} ${new_owner_log} ${new_owner_cmdline[@]}
}

run_test() {
  # Add the new owner service defined above
  services+=("${new_owner_service_name}")

  echo "⭐ Creating directories"
  create_directories

  echo "⭐ Generating service certificates"
  generate_certs

  echo "⭐ Build and install the 'go-fdo-client' binary"
  install_client

  echo "⭐ Build and install 'go-fdo-server' binary"
  install_server

  echo "⭐ Generating service configuration files"
  generate_service_configs

  echo "⭐ Start services"
  start_services

  echo "⭐ Wait for the services to be ready:"
  wait_for_services_ready

  echo "⭐ Setting or updating Rendezvous Info (RendezvousInfo)"
  set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

  echo "⭐ Run Device Initialization"
  run_device_initialization

  guid=$(get_device_guid "${device_credentials}")
  echo "⭐ Device initialized with GUID: ${guid}"

  echo "⭐ Sending Ownership Voucher to the Owner"
  send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

  echo "⭐ Trigger the Resell protocol on the current owner"
  resell "${owner_url}" "${guid}" "${new_owner_pub}" "${new_owner_ov}"

  echo "⭐ Sending the Ownership Voucher to the New Owner"
  send_ov_to_owner "${new_owner_url}" "${new_owner_ov}"

  echo "⭐ Setting or updating the New Owner Redirect Info (RVTO2Addr)"
  set_or_update_owner_redirect_info "${new_owner_url}" "${new_owner_service_name}" "${new_owner_dns}" "${new_owner_port}"

  echo "⭐ Triggering TO0 on the New Owner"
  run_to0 "${new_owner_url}" "${guid}" >/dev/null

  echo "⭐ Running FIDO Device Onboard"
  run_fido_device_onboard --debug

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
