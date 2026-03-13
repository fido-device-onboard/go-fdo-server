#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/cert-utils.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/fdo-utils.sh"


base_dir="${PWD}/test/workdir"
bin_dir="${base_dir}/bin"
configs_dir="${base_dir}/config"
databases_dir="${base_dir}/db"
pid_dir="${base_dir}/run"
log_level=debug
logs_dir="${base_dir}/logs"
certs_dir="${base_dir}/certs"
credentials_dir="${base_dir}/device-credentials"

device_ca_key="${certs_dir}/device_ca.key"
device_ca_crt="${device_ca_key/\.key/.crt}"
device_ca_subj="/C=US/O=FDO/CN=Device CA"

manufacturer_service_name="manufacturer"
manufacturer_dns=manufacturer
# needed for 'start_services' do not remove
#shellcheck disable=SC2034
manufacturer_ip=127.0.0.1
manufacturer_port=8038
manufacturer_database_type="sqlite"
manufacturer_database_dsn="file:${databases_dir}/manufacturer.db"
manufacturer_config_file="${configs_dir}/manufacturing.yaml"
manufacturer_pid_file="${pid_dir}/manufacturer.pid"
manufacturer_log_level="${log_level}"
manufacturer_log_file="${logs_dir}/${manufacturer_dns}.log"
# key crt pub and subj variables are required to generate certificates
manufacturer_key="${certs_dir}/manufacturer.key"
#shellcheck disable=SC2034
manufacturer_crt="${manufacturer_key/\.key/.crt}"
#shellcheck disable=SC2034
manufacturer_subj="/C=US/O=FDO/CN=Manufacturer"
manufacturer_service="${manufacturer_dns}:${manufacturer_port}"
# Default per-service protocol; caller may override
manufacturer_protocol=http
manufacturer_url="${manufacturer_protocol}://${manufacturer_service}"
#shellcheck disable=SC2034
# needed for 'wait_for_services_ready' do not remove
manufacturer_health_url="${manufacturer_url}/health"

rendezvous_service_name="rendezvous"
rendezvous_dns=rendezvous
#shellcheck disable=SC2034
# needed for 'start_services' do not remove
rendezvous_ip=127.0.0.1
rendezvous_port=8041
rendezvous_database_type="sqlite"
rendezvous_database_dsn="file:${databases_dir}/rendezvous.db"
rendezvous_config_file="${configs_dir}/rendezvous.yaml"
rendezvous_pid_file="${pid_dir}/rendezvous.pid"
rendezvous_log_level="${log_level}"
rendezvous_log_file="${logs_dir}/${rendezvous_dns}.log"
rendezvous_service="${rendezvous_dns}:${rendezvous_port}"
# Default per-service protocol; caller may override
rendezvous_protocol=http
rendezvous_url="${rendezvous_protocol}://${rendezvous_service}"
#shellcheck disable=SC2034
# needed for 'wait_for_services_ready' do not remove
rendezvous_health_url="${rendezvous_url}/health"

# Default RV info JSON for standard tests (can be overridden per test)
rv_info="[{\"dns\": \"${rendezvous_dns}\", \"device_port\": \"${rendezvous_port}\", \"protocol\": \"${rendezvous_protocol}\", \"ip\": \"${rendezvous_ip}\", \"owner_port\": \"${rendezvous_port}\"}]"

owner_service_name="owner"
owner_dns=owner
#shellcheck disable=SC2034
# needed for 'start_services' do not remove
owner_ip=127.0.0.1
owner_port=8043
owner_database_type="sqlite"
owner_database_dsn="file:${databases_dir}/owner.db"
owner_config_file="${configs_dir}/owner.yaml"
owner_pid_file="${pid_dir}/owner.pid"
owner_log_level="${log_level}"
owner_log_file="${logs_dir}/${owner_dns}.log"
# key crt pub and subj variables are required to generate certificates
owner_key="${certs_dir}/owner.key"
owner_crt="${owner_key/\.key/.crt}"
#shellcheck disable=SC2034
owner_pub="${owner_key/\.key/.pub}"
#shellcheck disable=SC2034
owner_subj="/C=US/O=FDO/CN=Owner"
owner_service="${owner_dns}:${owner_port}"
# Default per-service protocol; caller may override
owner_protocol=http
owner_url="${owner_protocol}://${owner_service}"
#shellcheck disable=SC2034
# needed for 'wait_for_services_ready' do not remove
owner_health_url="${owner_url}/health"
#shellcheck disable=SC2034
owner_ov="${base_dir}/owner.ov"

# HTTPS transport cert paths
manufacturer_https_key="${certs_dir}/manufacturer-http.key"
manufacturer_https_crt="${certs_dir}/manufacturer-http.crt"
rendezvous_https_key="${certs_dir}/rendezvous-http.key"
rendezvous_https_crt="${certs_dir}/rendezvous-http.crt"
owner_https_key="${certs_dir}/owner-http.key"
owner_https_crt="${certs_dir}/owner-http.crt"

# go-fdo-client operation timeout.
client_timeout="300s"

# seconds to wait for TO0 to complete
to0_wait_seconds=10

declare -a services=("${manufacturer_service_name}" "${rendezvous_service_name}" "${owner_service_name}")
declare -a directories=("${base_dir}" "${certs_dir}" "${credentials_dir}" "${logs_dir}" "${configs_dir}" "${databases_dir}")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
  echo -ne "$@"
}

log_info() {
  echo -e "${BLUE}[INFO]${NC} ⭐" "$@"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} 🚧" "$@"
}

log_success() {
  echo -e "✔" "$@"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} ❌" "$@"
  return 1
}

test_pass() {
  echo -e "${GREEN}[PASS]${NC} ✅ Test PASSED!"
}

test_fail() {
  log_error "Test FAILED!"
}

show_env() {
  env -0 | sort -z | tr '\0' '\n'
}

find_in_log() {
  local log=$1
  local pattern=$2
  grep -q "${pattern}" "${log}"
}

create_directories() {
  for directory in "${directories[@]}"; do
    mkdir -p "${directory}"
  done
}

set_hostnames() {
  for service in "${services[@]}"; do
    service_ip=${service}_ip
    service_dns=${service}_dns
    log "  ⚙ ${!service_ip} ${!service_dns} "
    set_hostname "${!service_dns}" "${!service_ip}"
    log_success
  done
}

set_hostname() {
  local dns=$1
  local ip=$2
  if grep -q " ${dns}" /etc/hosts; then
    tmp_hosts=$(mktemp)
    sed "s/.* ${dns}/$ip $dns/" /etc/hosts >"${tmp_hosts}"
    sudo cp "${tmp_hosts}" /etc/hosts
    rm -f "${tmp_hosts}"
  else
    echo "${ip} ${dns}" | sudo tee -a /etc/hosts >/dev/null
  fi
}

unset_hostnames() {
  log_info "Removing hostnames from '/etc/hosts'"
  for service in "${services[@]}"; do
    local service_ip=${service}_ip
    local service_dns=${service}_dns
    log "  ⚙ ${!service_ip} ${!service_dns} "
    unset_hostname "${!service_dns}" "${!service_ip}"
    log_success
  done
}

unset_hostname() {
  local dns=$1
  local ip=$2
  if grep -q " ${dns}" /etc/hosts; then
    tmp_hosts=$(mktemp)
    sed "/.* ${dns}/d" /etc/hosts >"${tmp_hosts}"
    sudo cp "${tmp_hosts}" /etc/hosts
    rm -f "${tmp_hosts}"
  fi
}

install_services() {
  log_info "Installing service"
  for service in "${services[@]}"; do
    log "  ⚙ Installing '${service}' "
    install_service ${service}
    log_success
  done
}

install_service() {
  local service=$1
  local install_service="install_service_${service}"
  ! declare -F "${install_service}" >/dev/null || ${install_service}
}

install_client() {
  go install github.com/fido-device-onboard/go-fdo-client@main
}

install_server() {
  mkdir -p "${bin_dir}"
  make build && install -m 755 go-fdo-server "${bin_dir}" && rm -f go-fdo-server
}

configure_services() {
  generate_https_certs
  for service in "${services[@]}"; do
    log "⚙ Configuring service ${service}\n"
    configure_service "${service}"
  done
}

configure_service() {
  local service=$1
  local configure_service="configure_service_${service}"
  ! declare -F "${configure_service}" >/dev/null || ${configure_service}
}

configure_service_manufacturer() {
  tee "${manufacturer_config_file}" <<EOF
log:
  level: "${manufacturer_log_level}"
db:
  type: "${manufacturer_database_type}"
  dsn: "${manufacturer_database_dsn}"
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

configure_service_rendezvous() {
  tee "${rendezvous_config_file}" <<EOF
log:
  level: "${rendezvous_log_level}"
db:
  type: "${rendezvous_database_type}"
  dsn: "${rendezvous_database_dsn}"
http:
  ip: "${rendezvous_dns}"
  port: ${rendezvous_port}
EOF
}

configure_service_owner() {
  tee "${owner_config_file}" <<EOF
log:
  level: "${owner_log_level}"
db:
  type: "${owner_database_type}"
  dsn: "${owner_database_dsn}"
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

start_services() {
  log_info "Adding hostnames to '/etc/hosts'"
  set_hostnames
  log_info "Starting Services"
  for service in "${services[@]}"; do
    log "  ⚙ Starting service ${service} "
    start_service ${service}
    log_success
  done
}

start_service() {
  local service=$1
  local start_service="start_service_${service}"
  ! declare -F "${start_service}" >/dev/null || ${start_service}
}

start_service_manufacturer() {
  local extra_opts=()
  if [ "${manufacturer_protocol}" = "https" ]; then
    extra_opts+=(--http-cert "${manufacturer_https_crt}" --http-key "${manufacturer_https_key}")
  fi
  run_go_fdo_server manufacturing \
    "${manufacturer_service}" \
    "${manufacturer_pid_file}" \
    "${manufacturer_log_level}" \
    "${manufacturer_log_file}" \
    "${manufacturer_database_type}" \
    "${manufacturer_database_dsn}" \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}" \
    "${extra_opts[@]}"
}

start_service_rendezvous() {
  local extra_opts=()
  if [ "${rendezvous_protocol}" = "https" ]; then
    extra_opts+=(--http-cert "${rendezvous_https_crt}" --http-key "${rendezvous_https_key}")
  fi
  run_go_fdo_server rendezvous \
    "${rendezvous_service}" \
    "${rendezvous_pid_file}" \
    "${rendezvous_log_level}" \
    "${rendezvous_log_file}" \
    "${rendezvous_database_type}" \
    "${rendezvous_database_dsn}"\
    "${extra_opts[@]}"
}

start_service_owner() {
  local extra_opts=()
  if [ "${owner_protocol}" = "https" ]; then
    extra_opts+=(--http-cert "${owner_https_crt}" --http-key "${owner_https_key}")
  fi
  if [ "${rendezvous_protocol}" = "https" ]; then
    # skip verify of rendezvous cert (self signed)
    extra_opts+=(--to0-insecure-tls)
  fi
  run_go_fdo_server owner \
    "${owner_service}" \
    "${owner_pid_file}" \
    "${owner_log_level}" \
    "${owner_log_file}" \
    "${owner_database_type}" \
    "${owner_database_dsn}" \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    "${extra_opts[@]}"
}

run_go_fdo_server() {
  local role=$1
  local address_port=$2
  local pid_file=$3
  local log_level=$4
  local log_file=$5
  local database_type=$6
  local database_dsn=$7
  shift 7
  mkdir -p "$(dirname "${log_file}")"
  mkdir -p "$(dirname "${pid_file}")"
  nohup "${bin_dir}/go-fdo-server" "${role}" "${address_port}" --db-type "${database_type}" --db-dsn "${database_dsn}" --log-level="${log_level}" "${@}" &>"${log_file}" &
  echo -n $! >"${pid_file}"
}


stop_services() {
  log_info "Stopping services"
  for service in "${services[@]}"; do
    log "  ⚙ Stopping service ${service} "
    stop_service "${service}"
    log_success
  done
}

stop_service() {
  local service=$1
  local service_pid_file="${service}_pid_file"
  if [[ -v "${service_pid_file}" ]] && [[ -f "${!service_pid_file}" ]]; then
    if pkill -F "${!service_pid_file}"; then
      wait "$(cat "${!service_pid_file}")" 2>/dev/null || :
    fi
  fi
}

uninstall_services() {
  log_info "Uninstalling service"
  for service in "${services[@]}"; do
    log "  ⚙ uninstalling service '${service}' "
    uninstall_service "${service}"
    log_success
  done
}

uninstall_service() {
  local service=$1
  local install_service="uninstall_service_${service}"
  ! declare -F "${install_service}" >/dev/null || ${install_service}
}

uninstall_client() {
  log_info "Uninstalling client"
  rm -vf "$(go env GOPATH)/bin/go-fdo-client"
}


uninstall_server() {
  log_info "Uninstalling server"
  rm -vf "${bin_dir}/go-fdo-server"
}

get_real_ip() {
  local service=$1
  local service_ip=${service}_ip
  echo "${!service_ip}"
}

wait_for_url() {
  local status
  local retry=0
  local -r interval=2
  local -r max_retries=5
  local url=$1
  while true; do
    [[ "$(curl --insecure --silent --output /dev/null --write-out '%{http_code}' "${url}")" = "200" ]] && break
    status=$?
    ((retry += 1))
    if [ $retry -gt $max_retries ]; then
      return $status
    fi
    sleep "$interval"
  done
}

wait_for_service_ready() {
  local service=$1
  local service_health_url="${service}_health_url"
  [[ -v "${service_health_url}" ]] || log_error "service ${service} has no health URL"
  log "  ⚙ Waiting for ${!service_health_url} to be healthy "
  wait_for_url "${!service_health_url}" || {
    status="$?"
    echo "❌"
    return "${status}"
  }
  log_success
}

wait_for_services_ready() {
  for service in "${services[@]}"; do
    # only wait for those services that define a health URL
    local service_health_url="${service}_health_url"
    [[ ! -v "${service_health_url}" ]] || wait_for_service_ready "${service}"
  done
}

run_go_fdo_client() {
  mkdir -p "${credentials_dir}"
  cd "${credentials_dir}"
  # If the command times out, the return code is 124 (see: man timeout)
  # If the command finishes before the timeout, the return code comes from 'go-fdo-client'
  local exit_code=0
  timeout "${client_timeout}" go-fdo-client "$@" || exit_code=$?
  if [[ ${exit_code} -ne 0 ]]; then
    log_warn "'go-fdo-client' exited with '${exit_code}' (124 -> timeout):\n  - go-fdo-client $*"
  fi
  cd - >/dev/null
  return ${exit_code}
}

run_device_initialization() {
  local tmp_credentials_file="$(mktemp --tmpdir="${credentials_dir}")"
  local tmp_log_file="$(mktemp --tmpdir="${logs_dir}")"}
  run_go_fdo_client --blob "${tmp_credentials_file}" --debug device-init "${manufacturer_url}" --device-info=gotest --key ec256 --insecure-tls=true | tee -a ${tmp_log_file} >&2
  guid=$(run_go_fdo_client --blob "${tmp_credentials_file}" print | grep GUID | awk '{print $2}')
  local device_credentials="$(get_device_credentials_file_path "${guid}")"
  local log_file="$(get_device_onboard_log_file_path "${guid}")"
  mv "${tmp_credentials_file}" "${device_credentials}"
  mv "${tmp_log_file}" "${log_file}"
  echo "${guid}"
}

get_device_init_log_file_path() {
  guid=${1}
  [[ "${guid}" =~ ^[a-f0-9]{32}$ ]] || log_error "Device guid required as first argument"
  echo "${logs_dir}/${guid}-device-onboard.log"
}

get_device_credentials_file_path() {
  guid=${1}
  [[ "${guid}" =~ ^[a-f0-9]{32}$ ]] || log_error "Device guid required as first argument"
  echo "${credentials_dir}/${guid}.creds"
}

get_device_onboard_log_file_path() {
  guid=${1}
  [[ "${guid}" =~ ^[a-f0-9]{32}$ ]] || log_error "Device guid required as first argument"
  [ -n "${guid}" ] || log_error "Device guid required as first argument"
  echo "${logs_dir}/${guid}-device-onboard.log"
}

run_fido_device_onboard() {
  local guid=$1
  local log_file
  shift
  [[ "${guid}" =~ ^[a-f0-9]{32}$ ]] || log_error "Device guid required as first argument"
  log_file="$(get_device_onboard_log_file_path "${guid}")"
  run_go_fdo_client --blob "$(get_device_credentials_file_path "${guid}")" onboard --key ec256 --kex ECDH256 --insecure-tls=true "$@" | tee -a "${log_file}"
  find_in_log "${log_file}" 'FIDO Device Onboard Complete' || return $?
}

generate_service_certs() {
  for service in "${services[@]}"; do
    local service_key="${service}_key"
    local service_crt="${service}_crt"
    local service_subj="${service}_subj"
    if [[ -v "${service_key}" && -v "${service_crt}" && -v "${service_subj}" ]]; then
      generate_cert "${!service_key}" "${!service_crt}" "${!service_subj}"
    fi
  done
  generate_cert "${device_ca_key}" "${device_ca_crt}" "${device_ca_subj}"
  ls -l "${certs_dir}"
}

generate_https_certs() {
  for service in "${services[@]}"; do
    local service_protocol="${service}_protocol"
    [[ "${!service_protocol-}" = "https" ]] || continue
    local service_key="${service}_https_key"
    local service_crt="${service}_https_crt"
    local service_dns="${service}_dns"
    local service_subj="/C=US/O=FDO/CN=${!service_dns:-${service}}"
    if [[ -v "${service_key}" && -v "${service_crt}" ]]; then
      generate_cert "${!service_key}" "${!service_crt}" "${service_subj}" "pem"
    fi
  done
}

set_or_update_rendezvous_info() {
  local manufacturer_url=$1
  local rendezvous_info_json=$2

  log_info "Checking if 'RendezvousInfo' is configured on manufacturer side (${manufacturer_url})"
  if [ -z "$(get_rendezvous_info "${manufacturer_url}")" ]; then
    log_warn "'RendezvousInfo' not found, creating it"
    set_rendezvous_info "${manufacturer_url}" "${rendezvous_info_json}"
  else
    log_info "'RendezvousInfo' found, updating it"
    update_rendezvous_info "${manufacturer_url}" "${rendezvous_info_json}"
  fi
  echo
}

send_manufacturer_ov_to_owner() {
  local manufacturer_url=$1
  local guid=$2
  local owner_url=$3
  ov_dir="${base_dir}/ovs"
  mkdir -p "${ov_dir}"
  ov_file="${ov_dir}/${guid}.ov"
  get_ov_from_manufacturer "${manufacturer_url}" "${guid}" "${ov_file}"
  send_ov_to_owner "${owner_url}" "${ov_file}"
  status=$?
  log_info "Waiting '${to0_wait_seconds}' seconds for TO0 to be performed by the Owner"
  sleep "${to0_wait_seconds}"
  return ${status}
}

set_or_update_owner_redirect_info() {
  local owner_url=$1
  local owner_service_name=$2
  local owner_dns=$3
  local owner_port=$4
  local owner_protocol=${5:-http}
  local real_owner_ip
  real_owner_ip="$(get_real_ip "${owner_service_name}")"
  log_info "Checking if 'RVTO2Addr' is configured on owner side (${owner_url})"
  if [ -z "$(get_owner_redirect_info "${owner_url}")" ]; then
    log_warn "'RVTO2Addr' not found, creating it"
    set_owner_redirect_info "${owner_url}" "${real_owner_ip}" "${owner_dns}" "${owner_port}" "${owner_protocol}"
  else
    log_info "'RVTO2Addr' found, updating it"
    update_owner_redirect_info "${owner_url}" "${real_owner_ip}" "${owner_dns}" "${owner_port}" "${owner_protocol}"
  fi
  echo
}

get_service_logs() {
  local service=$1
  local service_log_var="${service}_log_file"
  if [[ -v "${service_log_var}" ]]; then
    [ ! -f "${!service_log_var}" ] || cat "${!service_log_var}"
  fi
}

get_logs() {
  log_info "Retrieving logs"
  for service in "${services[@]}"; do
    log "🛑 '${service}' logs:\n"
    get_service_logs "${service}"
  done
}

prepare_payload() {
  local file_path=$1
  mkdir -p "$(dirname "${file_path}")"
  dd if=/dev/urandom of="${file_path}" bs=1M count=2 2>/dev/null
}

verify_equal_files() {
  local file_1=$1
  local file_2=$2

  for file in "${file_1}" "${file_2}"; do
    [ -f "${file}" ] || log_error "File not found: ${file}"
  done

  [ "${file_1}" != "${file_2}" ] || return 0

  local file_1_sha file_2_sha
  file_1_sha=$(sha256sum "${file_1}" | awk '{print $1}')
  file_2_sha=$(sha256sum "${file_2}" | awk '{print $1}')
  if [ "${file_1_sha}" != "${file_2_sha}" ]; then
    log_error "Checksum mismatch: ${file_1}=${file_1_sha} ${file_2}=${file_2_sha}"
  fi
}

on_failure() {
  trap - ERR
  stop_services
  test_fail
}

remove_files() {
  log_info "Removing files from '${base_dir:?}'"
  rm -vrf "${base_dir:?}"/*
}

cleanup() {
  stop_services
  unset_hostnames
  uninstall_services
  remove_files
}
