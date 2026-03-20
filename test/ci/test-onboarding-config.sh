#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-onboarding.sh"

# override to remove use of CLI flags
run_go_fdo_server() {
  local role=$1
  local pid_file=$2
  local log=$3
  shift 3
  mkdir -p "$(dirname "${log}")"
  mkdir -p "$(dirname "${pid_file}")"
  nohup "${bin_dir}/go-fdo-server" "${role}" "${@}" &>"${log}" &
  echo -n $! >"${pid_file}"
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

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
