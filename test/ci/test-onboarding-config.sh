#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

configs_dir="${base_dir}/configs"
manufacturer_config_file="${configs_dir}/manufacturing.yaml"
rendezvous_config_file="${configs_dir}/rendezvous.yaml"
owner_config_file="${configs_dir}/owner.yaml"

directories+=("${configs_dir}")

generate_manufacturer_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "sqlite"
  dsn: "file:${base_dir}/manufacturer.db"
http:
  ip: "${manufacturer_dns}"
  port: ${manufacturer_port}
manufacturing:
  key: "${manufacturer_key}"
device_ca:
  cert: "${device_ca_crt}"
  key: "${device_ca_key}"
owner:
  cert: "${owner_crt}"
EOF
}

generate_rendezvous_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "sqlite"
  dsn: "file:${base_dir}/rendezvous.db"
http:
  ip: "${rendezvous_dns}"
  port: ${rendezvous_port}
EOF
}

generate_owner_config() {
  cat <<EOF
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
EOF
}

generate_service_configs() {
  for service in "${services[@]}"; do
    local gen_func="generate_${service}_config"
    local conf_file="${service}_config_file"
    if declare -F "${gen_func}" > /dev/null; then
      "${gen_func}" > "${!conf_file}"
    fi
  done
}

# override to remove use of CLI flags
run_go_fdo_server() {
  local role=$1
  local pid_file=$2
  local log=$3
  shift 3
  mkdir -p "$(dirname "${log}")"
  mkdir -p "$(dirname "${pid_file}")"
  nohup "${bin_dir}/go-fdo-server" "${role}" "${@}" &> "${log}" &
  echo -n $! > "${pid_file}"
}

start_service_manufacturer() {
  run_go_fdo_server manufacturing ${manufacturer_pid_file} ${manufacturer_log} \
                    --config=${manufacturer_config_file}
}

start_service_rendezvous() {
  run_go_fdo_server rendezvous ${rendezvous_pid_file} ${rendezvous_log} \
                    --config=${rendezvous_config_file}
}

start_service_owner() {
  run_go_fdo_server owner ${owner_pid_file} ${owner_log} \
                    --config=${owner_config_file}
}

run_test() {

  echo "⭐ Creating directories"
  create_directories

  echo "⭐ Generating service certificates"
  generate_service_certs

  echo "⭐ Build and install 'go-fdo-client' binary"
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

  echo "⭐ Success! ✅"
  trap cleanup EXIT
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
