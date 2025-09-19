#! /bin/bash

set -xeuo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../scripts/cert-utils.sh"
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../scripts/fdo-utils.sh"

base_dir=/tmp/go-fdo
bin_dir=${base_dir}/bin
creds_dir=${base_dir}/device-credentials
device_credentials=${creds_dir}/creds.bin

certs_dir=${base_dir}/certs

manufacturer_dns=manufacturer
manufacturer_ip=127.0.0.1
manufacturer_port=8038
manufacturer_log="${base_dir}/${manufacturer_dns}.log"
manufacturer_key="${certs_dir}/manufacturer.key"
manufacturer_crt="${manufacturer_key/\.key/.crt}"
manufacturer_pub="${manufacturer_key/\.key/.pub}"
device_ca_key="${certs_dir}/device-ca.key"
device_ca_crt="${device_ca_key/\.key/.crt}"
device_ca_pub="${device_ca_key/\.key/.pub}"

rendezvous_dns=rendezvous
rendezvous_ip=127.0.0.1
rendezvous_port=8041
rendezvous_log="${base_dir}/${rendezvous_dns}.log"

owner_dns=owner
owner_ip=127.0.0.1
owner_port=8043
owner_log="${base_dir}/${owner_dns}.log"
owner_onboard_log="${base_dir}/onboarding-${owner_dns}.log"
owner_ov="${base_dir}/owner.ov"
owner_key="${certs_dir}/owner.key"
owner_crt="${owner_key/\.key/.crt}"
owner_pub="${owner_key/\.key/.pub}"

new_owner_dns=new-owner
new_owner_ip=127.0.0.1
new_owner_port=8045
new_owner_log="${base_dir}/${new_owner_dns}.log"
new_owner_onboard_log="${base_dir}/onboarding-${owner_dns}.log"
new_owner_ov="${base_dir}/new-owner.ov"
new_owner_key="${certs_dir}/new-owner.key"
new_owner_crt="${new_owner_key/\.key/.crt}"
new_owner_pub="${new_owner_key/\.key/.pub}"

manufacturer_service="${manufacturer_dns}:${manufacturer_port}"
rendezvous_service="${rendezvous_dns}:${rendezvous_port}"
owner_service="${owner_dns}:${owner_port}"
new_owner_service="${new_owner_dns}:${new_owner_port}"

setup_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  if grep -q " ${dns}" /etc/hosts ; then
    sudo sed -i "s/.* ${dns}/$ip $dns/" /etc/hosts
  else
    sudo echo "${ip} ${dns}" | sudo tee -a /etc/hosts;
  fi
}

unset_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  ! grep -q " ${dns}" /etc/hosts || sudo sed -ie "/.* ${dns}/d" /etc/hosts
}

setup_hostnames () {
  setup_hostname ${manufacturer_dns} ${manufacturer_ip}
  setup_hostname ${rendezvous_dns} ${rendezvous_ip}
  setup_hostname ${owner_dns} ${owner_ip}
  setup_hostname ${new_owner_dns} ${new_owner_ip}
}

unset_hostnames () {
  unset_hostname ${manufacturer_dns} ${manufacturer_ip}
  unset_hostname ${rendezvous_dns} ${rendezvous_ip}
  unset_hostname ${owner_dns} ${owner_ip}
  unset_hostname ${new_owner_dns} ${new_owner_ip}
}

update_ips() {
  # Only needed for container tests
  echo "Not needed"
  return
}

wait_for_service() {
    local status
    local retry=0
    local -r interval=2
    local -r max_retries=5
    local service=$1
    echo "Waiting for ${service} to be healthy"
    while true; do
        [[ "$(curl --silent --output /dev/null --write-out '%{http_code}' "http://${service}/health")" = "200" ]] && break
        status=$?
        ((retry+=1))
        if [ $retry -gt $max_retries ]; then
            return $status
        fi
        echo "info: Waiting for a while, then retry ..." 1>&2
        sleep "$interval"
    done
}

wait_for_fdo_servers_ready () {
  # manufacturer server
  wait_for_service "${manufacturer_service}"
  # Rendezvous server
  wait_for_service "${rendezvous_service}"
  # Owner server
  wait_for_service "${owner_service}"
  # New Owner server
  wait_for_service "${new_owner_service}"
}

run_device_initialization() {
  mkdir -p "${creds_dir}"
  cd ${creds_dir}
  rm -f "${device_credentials}"
  go-fdo-client --blob "${device_credentials}" --debug device-init "http://${manufacturer_service}" --device-info=gotest --key ec256
  cd -
}

get_device_guid () {
  go-fdo-client --blob "${device_credentials}" --debug print | grep GUID | awk '{print $2}'
}

run_fido_device_onboard () {
  local log=$1
  local extra_args=("${@:2}")
  cd ${creds_dir}
  go-fdo-client --blob "${device_credentials}" --debug onboard --key ec256 --kex ECDH256 "${extra_args[@]}" | tee "${log}"
  cd -
  grep 'FIDO Device Onboard Complete' "${log}"
}

get_server_logs() {
  [ ! -f "${manufacturer_log}" ] || cat "${manufacturer_log}"
  [ ! -f "${rendezvous_log}" ]    || cat "${rendezvous_log}"
  [ ! -f "${owner_log}" ]         || cat "${owner_log}"
  [ ! -f "${new_owner_log}" ]     || cat "${new_owner_log}"
}

run_service () {
  local role=$1
  local address_port=$2
  local db=$3
  local log=$4
  nohup ${bin_dir}/go-fdo-server "${role}" "${address_port}" --db "${base_dir}/${db}.sqlite" --db-pass '2=,%95QF<uTLLHt' --debug "${@:5}" &> "${log}" &
}

run_services () {
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}
  run_service owner ${owner_service} owner ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}"
  run_service owner ${new_owner_service} new_owner ${new_owner_log} \
    --owner-key="${new_owner_key}" \
    --device-ca-cert="${device_ca_crt}"
}

stop_services () {
  killall -q go-fdo-server || :
}

install_client() {
  go install github.com/fido-device-onboard/go-fdo-client@latest
}

uninstall_client() {
  rm -rf "$(go env GOPATH)/bin/go-fdo-client"
}

install_server() {
  mkdir -p ${bin_dir}
  make && install -D -m 755 -t ${bin_dir} go-fdo-server  && rm -f go-fdo-server
}

uninstall_server() {
  rm -f ${bin_dir}/go-fdo-server
}

generate_certs() {
  mkdir -p "${certs_dir}"
  generate_cert "${manufacturer_key}" "${manufacturer_crt}" "${manufacturer_pub}" "/C=US/O=FDO/CN=Manufacturer"
  generate_cert "${device_ca_key}" "${device_ca_crt}" "${device_ca_pub}" "/C=US/O=FDO/CN=Device CA"
  generate_cert "${owner_key}" "${owner_crt}" "${owner_pub}" "/C=US/O=FDO/CN=Owner"
  generate_cert "${new_owner_key}" "${new_owner_crt}" "${new_owner_pub}" "/C=US/O=FDO/CN=New Owner"
  chmod a+r "${certs_dir}"/*
  ls -l "${certs_dir}"
}

setup_env() {
  mkdir -p ${base_dir}
  setup_hostnames
  run_services
  wait_for_fdo_servers_ready
  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
}

test_onboarding () {
  update_ips
  update_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log}
}

test_resale() {
  update_ips
  update_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  send_ov_to_owner ${owner_service} ${owner_ov}
  resell ${owner_service} "${guid}" "${new_owner_pub}" ${new_owner_ov}
  set_owner_redirect_info ${new_owner_service} ${new_owner_ip} ${new_owner_port}
  send_ov_to_owner ${new_owner_service} ${new_owner_ov}
  run_to0 ${new_owner_service} "${guid}"
  run_fido_device_onboard ${new_owner_onboard_log}
}

cleanup() {
  stop_services
  uninstall_server
  uninstall_client
  unset_hostnames
  rm -rf ${base_dir}
}

test_all () {
  echo "======================== Make sure the env is clean ========================================="
  cleanup
  echo "======================== Generating service certificates ===================================="
  generate_certs
  echo "======================== Install 'go-fdo-client' binary ====================================="
  install_client
  echo "======================== Install 'go-fdo-server' binary ====================================="
  install_server
  echo "======================== Configure the environment  ========================================="
  setup_env
  echo "======================== Testing FDO Onboarding  ============================================"
  test_onboarding
  echo "======================== Testing FDO Resale protocol ========================================"
  test_resale
  echo "======================== Clean the environment =============================================="
  cleanup
}
