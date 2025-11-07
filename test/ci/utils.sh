#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/cert-utils.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/fdo-utils.sh"

base_dir="${PWD}/test/workdir"
bin_dir="${base_dir}/bin"
pid_dir="${base_dir}/run"
logs_dir="${base_dir}/logs"
certs_dir="${base_dir}/certs"
credentials_dir="${base_dir}/device-credentials"
device_credentials="${credentials_dir}/creds.bin"

device_ca_key="${certs_dir}/device_ca.key"
device_ca_crt="${device_ca_key/\.key/.crt}"
device_ca_pub="${device_ca_key/\.key/.pub}"
device_ca_subj="/C=US/O=FDO/CN=Device CA"

manufacturer_service_name="manufacturer"
manufacturer_dns=manufacturer
# needed for 'start_services' do not remove
#shellcheck disable=SC2034
manufacturer_ip=127.0.0.1
manufacturer_port=8038
manufacturer_pid_file="${pid_dir}/manufacturer.pid"
manufacturer_log="${logs_dir}/${manufacturer_dns}.log"
# key crt pub and subj variables are required to generate certificates
manufacturer_key="${certs_dir}/manufacturer.key"
#shellcheck disable=SC2034
manufacturer_crt="${manufacturer_key/\.key/.crt}"
#shellcheck disable=SC2034
manufacturer_pub="${manufacturer_key/\.key/.pub}"
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
rendezvous_pid_file="${pid_dir}/rendezvous.pid"
rendezvous_log="${logs_dir}/${rendezvous_dns}.log"
rendezvous_service="${rendezvous_dns}:${rendezvous_port}"
# Default per-service protocol; caller may override
rendezvous_protocol=http
rendezvous_url="${rendezvous_protocol}://${rendezvous_service}"
#shellcheck disable=SC2034
# needed for 'wait_for_services_ready' do not remove
rendezvous_health_url="${rendezvous_url}/health"

owner_service_name="owner"
owner_dns=owner
#shellcheck disable=SC2034
# needed for 'start_services' do not remove
owner_ip=127.0.0.1
owner_port=8043
owner_pid_file="${pid_dir}/owner.pid"
owner_log="${logs_dir}/${owner_dns}.log"
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

# Define HTTPS certificate subjects per service (passed down to cert generator)
manufacturer_https_subj="/C=US/O=FDO/CN=manufacturer"
rendezvous_https_subj="/C=US/O=FDO/CN=rendezvous"
owner_https_subj="/C=US/O=FDO/CN=owner"

# HTTPS transport cert paths
manufacturer_https_key="${certs_dir}/manufacturer-http.key"
manufacturer_https_crt="${certs_dir}/manufacturer-http.crt"
rendezvous_https_key="${certs_dir}/rendezvous-http.key"
rendezvous_https_crt="${certs_dir}/rendezvous-http.crt"
owner_https_key="${certs_dir}/owner-http.key"
owner_https_crt="${certs_dir}/owner-http.crt"

declare -a services=("${manufacturer_service_name}" "${rendezvous_service_name}" "${owner_service_name}")
declare -a directories=("${base_dir}" "${certs_dir}" "${credentials_dir}" "${logs_dir}")

find_in_log_or_fail() {
  local log=$1
  local pattern=$2
  grep -q "${pattern}" "${log}" || {
    echo "❌ '${pattern}' not found in '${log}' "
    return 1
  }
}

create_directories() {
  for directory in "${directories[@]}"; do
    mkdir -p "${directory}"
  done
}

set_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  if grep -q " ${dns}" /etc/hosts; then
    echo "${ip} ${dns}"
    tmp_hosts=$(mktemp)
    sed "s/.* ${dns}/$ip $dns/" /etc/hosts >"${tmp_hosts}"
    sudo cp "${tmp_hosts}" /etc/hosts
    rm -f "${tmp_hosts}"
  else
    echo "${ip} ${dns}" | sudo tee -a /etc/hosts
  fi
}

unset_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  echo "${ip} ${dns}"
  if grep -q " ${dns}" /etc/hosts; then
    tmp_hosts=$(mktemp)
    sed "/.* ${dns}/d" /etc/hosts >"${tmp_hosts}"
    sudo cp "${tmp_hosts}" /etc/hosts
    rm -f "${tmp_hosts}"
  fi
}

set_hostnames() {
  echo "⭐ Adding hostnames to '/etc/hosts'"
  for service in "${services[@]}"; do
    service_ip=${service}_ip
    service_dns=${service}_dns
    set_hostname "${!service_dns}" "${!service_ip}"
  done
}

unset_hostnames() {
  echo "⭐ Removing hostnames from '/etc/hosts'"
  for service in "${services[@]}"; do
    local service_ip=${service}_ip
    local service_dns=${service}_dns
    unset_hostname "${!service_dns}" "${!service_ip}"
  done
}

configure_services() {
  generate_https_certs
  echo "⭐ Configuring services"
  for service in "${services[@]}"; do
    configure_service "${service}"
  done
}

configure_service() {
  local service=$1
  echo "  ⚙ Configuring service ${service}"
  local configure_service="configure_service_${service}"
  ! declare -F "${configure_service}" >/dev/null || ${configure_service}
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
  echo "url: ${url}"
  echo -n "❓ Waiting for ${url} to be healthy "
  while true; do
    [[ "$(curl --insecure --silent --output /dev/null --write-out '%{http_code}' "${url}")" = "200" ]] && break
    status=$?
    ((retry += 1))
    if [ $retry -gt $max_retries ]; then
      echo " ❌"
      return $status
    fi
    echo -n "." 1>&2
    sleep "$interval"
  done
  echo " ✔"
}

wait_for_service_ready() {
  local service=$1
  local service_health_url="${service}_health_url"
  [[ -v "${service_health_url}" ]] || {
    echo "❌ service ${service} has no health URL"
    return 1
  }
  wait_for_url "${!service_health_url}"
}

wait_for_services_ready() {
  for service in "${services[@]}"; do
    # only wait for those services that define a health URL
    local service_health_url="${service}_health_url"
    [[ ! -v "${service_health_url}" ]] || wait_for_service_ready "${service}"
  done
}

run_go_fdo_client() {
  mkdir -p ${credentials_dir}
  cd ${credentials_dir}
  go-fdo-client "$@"
  cd - >/dev/null
}

run_device_initialization() {
  [ ! -f "${device_credentials}" ] || rm -f "${device_credentials}"
  run_go_fdo_client --blob "${device_credentials}" --debug device-init "${manufacturer_url}" --device-info=gotest --key ec256 --insecure-tls=true
}

get_device_guid() {
  run_go_fdo_client --blob "${device_credentials}" print | grep GUID | awk '{print $2}'
}

get_device_onboard_log() {
  echo "${logs_dir}/onboarding-device-$(get_device_guid).log"
}

run_fido_device_onboard() {
  log="$(get_device_onboard_log)"
  >"${log}"
  # This logic will be removed once the go-fdo-client supports polling for TO2 completion.
  local attempts=5
  local i=1
  while [ ${i} -le ${attempts} ]; do
    run_go_fdo_client --blob "${device_credentials}" onboard --key ec256 --kex ECDH256 --insecure-tls=true "$@" | tee -a "${log}"
    if grep -q 'FIDO Device Onboard Complete' "${log}"; then
      break
    fi
    if [ ${i} -lt ${attempts} ]; then
      sleep 10
    fi
    i=$((i+1))
  done
  find_in_log_or_fail "${log}" 'FIDO Device Onboard Complete'
}

run_go_fdo_server() {
  local role=$1
  local address_port=$2
  local name=$3
  local pid_file=$4
  local log=$5
  shift 5
  mkdir -p "$(dirname "${log}")"
  mkdir -p "$(dirname "${pid_file}")"
  nohup "${bin_dir}/go-fdo-server" "${role}" "${address_port}" --db-type sqlite --db-dsn "file:${base_dir}/${name}.db" --log-level=debug "${@}" &> "${log}" &
  echo -n $! > "${pid_file}"
}

start_service_manufacturer() {
  local extra_opts=()
  if [ "${manufacturer_protocol}" = "https" ]; then
    extra_opts+=(--http-cert "${manufacturer_https_crt}" --http-key "${manufacturer_https_key}")
  fi
  run_go_fdo_server manufacturing ${manufacturer_service} manufacturer ${manufacturer_pid_file} ${manufacturer_log} \
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
  run_go_fdo_server rendezvous ${rendezvous_service} rendezvous ${rendezvous_pid_file} ${rendezvous_log} \
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
  run_go_fdo_server owner ${owner_service} owner ${owner_pid_file} ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    "${extra_opts[@]}"
}

start_service() {
  local service=$1
  echo -n "  ⚙ Starting service ${service} "
  local start_service="start_service_${service}"
  ! declare -F "${start_service}" >/dev/null || ${start_service}
  echo " ✔"
}

start_services() {
  set_hostnames
  echo "⭐ Starting services"
  for service in "${services[@]}"; do
    start_service ${service}
  done
}

stop_service() {
  local service=$1
  local service_pid_file="${service}_pid_file"
  echo -n "  ⚙ Stopping service ${service} "
  if [[ -v "${service_pid_file}" ]] && [[ -f "${!service_pid_file}" ]]; then
    if pkill -F "${!service_pid_file}"; then
      wait "$(cat ${!service_pid_file})" 2>/dev/null || :
    fi
  fi
  echo " ✔"
}

stop_services() {
  echo "⭐ Stopping services"
  for service in "${services[@]}"; do
    stop_service ${service}
  done
}

install_client() {
  go install github.com/fido-device-onboard/go-fdo-client@latest
}

uninstall_client() {
  echo "⭐ Uninstalling client"
  rm -rf "$(go env GOPATH)/bin/go-fdo-client"
}

install_server() {
  mkdir -p "${bin_dir}"
  make build && install -m 755 go-fdo-server ${bin_dir} && rm -f go-fdo-server
}

uninstall_server() {
  echo "⭐ Uninstalling server"
  rm -f "${bin_dir}/go-fdo-server"
}

generate_certs() {
  for service in "${services[@]}"; do
    local service_key="${service}_key"
    local service_crt="${service}_crt"
    local service_pub="${service}_pub"
    local service_subj="${service}_subj"
    if [[ -v "${service_key}" && -v "${service_crt}" && -v "${service_pub}" && -v "${service_subj}" ]]; then
      generate_cert "${!service_key}" "${!service_crt}" "${!service_pub}" "${!service_subj}"
    fi
  done
  generate_cert "${device_ca_key}" "${device_ca_crt}" "${device_ca_pub}" "${device_ca_subj}"
  ls -l "${certs_dir}"
}

set_or_update_rendezvous_info() {
  local manufacturer_url=$1
  local rendezvous_service_name=$2
  local rendezvous_dns=$3
  local rendezvous_port=$4
  
  local real_rendezvous_ip
  real_rendezvous_ip="$(get_real_ip "${rendezvous_service_name}")"
  echo "❓ Checking if 'RendezvousInfo' is configured on manufacturer side (${manufacturer_url})"
  if [ -z "$(get_rendezvous_info "${manufacturer_url}")" ]; then
    echo "🚧 'RendezvousInfo' not found, creating it..."
    set_rendezvous_info "${manufacturer_url}" "${rendezvous_dns}" "${real_rendezvous_ip}" "${rendezvous_port}" "${rendezvous_protocol}"
  else
    echo "⚙ 'RendezvousInfo; found, updating it..."
    update_rendezvous_info "${manufacturer_url}" "${rendezvous_dns}" "${real_rendezvous_ip}" "${rendezvous_port}" "${rendezvous_protocol}"
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
}

set_or_update_owner_redirect_info() {
  local owner_url=$1
  local owner_service_name=$2
  local owner_dns=$3
  local owner_port=$4
  local real_owner_ip
  real_owner_ip="$(get_real_ip "${owner_service_name}")"
  echo "❓ Checking if 'RVTO2Addr' is configured on owner side (${owner_url})"
  if [ -z "$(get_owner_redirect_info "${owner_url}")" ]; then
    echo "🚧 'RVTO2Addr' not found, creating it..."
    set_owner_redirect_info "${owner_url}" "${real_owner_ip}" "${owner_dns}" "${owner_port}" "${owner_protocol}"
  else
    echo "⚙ 'RVTO2Addr' found, updating it..."
    update_owner_redirect_info "${owner_url}" "${real_owner_ip}" "${owner_dns}" "${owner_port}" "${owner_protocol}"
  fi
  echo
}

get_service_logs() {
  local service=$1
  local service_log_var="${service}_log"
  if [[ -v "${service_log_var}" ]]; then
    echo "🛑 ❓ '${service}' logs:"
    [ ! -f "${!service_log_var}" ] || cat "${!service_log_var}"
  fi
}

get_logs() {
  echo "⭐ Retrieving logs"
  for service in "${services[@]}"; do
    get_service_logs ${service}
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
    [ -f "${file}" ] || {
      echo "❌ File not found: ${file}"
      return 1
    }
  done

  [ "${file_1}" != "${file_2}" ] || return 0

  local file_1_sha file_2_sha
  file_1_sha=$(sha256sum "${file_1}" | awk '{print $1}')
  file_2_sha=$(sha256sum "${file_2}" | awk '{print $1}')
  if [ "${file_1_sha}" != "${file_2_sha}" ]; then
    echo "❌ Checksum mismatch: ${file_1}=${file_1_sha} ${file_2}=${file_2_sha}"
    return 1
  fi
}

on_failure() {
  trap - ERR
  stop_services
  echo "❌ Test FAILED!"
}

remove_files() {
  echo "⭐ Removing files from '${base_dir:?}'"
  rm -rf "${base_dir:?}"/*
}

cleanup() {
  echo "⭐ Cleaning ..."
  stop_services
  unset_hostnames
  uninstall_server
  uninstall_client
  remove_files
  echo "⭐ Done!"
}
