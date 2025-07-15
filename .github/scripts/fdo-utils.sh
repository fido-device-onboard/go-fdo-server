#! /bin/bash

set -xeuo pipefail

manufacturer_dns=manufacturer
manufacturer_ip=127.0.0.1
manufacturer_port=8038
manufacturer_log="/tmp/${manufacturer_dns}.log"

rendezvous_dns=rendezvous
rendezvous_ip=127.0.0.1
rendezvous_port=8041
rendezvous_log="/tmp/${rendezvous_dns}.log"

owner_dns=owner
owner_ip=127.0.0.1
owner_port=8043
owner_log="/tmp/${owner_dns}.log"

manufacturer_service="${manufacturer_dns}:${manufacturer_port}"
rendezvous_service="${rendezvous_dns}:${rendezvous_port}"
owner_service="${owner_dns}:${owner_port}"

setup_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  if grep -q "${dns}" /etc/hosts ; then
    sudo sed -ie "s/.*${dns}/$ip $dns/" /etc/hosts
  else
    sudo echo "${ip} ${dns}" | sudo tee -a /etc/hosts;
  fi
}

setup_hostnames () {
  setup_hostname ${manufacturer_dns} ${manufacturer_ip}
  setup_hostname ${rendezvous_dns} ${rendezvous_ip}
  setup_hostname ${owner_dns} ${owner_ip}
}

wait_for_service() {
    local status
    local retry=0
    local -r interval=2
    local -r max_retries=1005
    local service=$1
    echo "Waiting for ${service} to be healthy"
    while true; do
        test "$(curl --silent --output /dev/null --write-out '%{http_code}' "http://${service}/health")" = "200" && break
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
  # Manufacturer server
  wait_for_service "${manufacturer_service}"
  # Rendezvous server
  wait_for_service "${rendezvous_service}"
  # Owner server
  wait_for_service "${owner_service}"
}

set_rendezvous_info () {
    curl --fail --verbose --silent \
         --header 'Content-Type: text/plain' \
         --data-raw "[[[5,\"${rendezvous_dns}\"],[3,8041],[12,1],[2,\"${rendezvous_ip}\"],[4,8041]]]" \
         "http://${manufacturer_service}/api/v1/rvinfo"
}

run_device_initialization() {
  go-fdo-client --blob creds.bin --debug device-init "http://${manufacturer_service}" --device-info=gotest --key ec256
}

send_voucher_to_owner () {
  local guid
  guid=$(go-fdo-client --blob creds.bin --debug print | grep GUID | awk '{print $2}')
  curl --fail --verbose --silent "http://${manufacturer_service}/api/v1/vouchers?guid=${guid}" -o ownervoucher
  curl --fail --verbose --silent "http://${owner_service}/api/v1/owner/vouchers" -d @ownervoucher
}

run_to0 () {
  local guid
  guid=$(go-fdo-client --blob creds.bin --debug print | grep GUID | awk '{print $2}')
  curl --fail --verbose --silent "http://${owner_service}/api/v1/to0/${guid}"
}

run_fido_device_onboard () {
  go-fdo-client --blob creds.bin --debug onboard --key ec256 --kex ECDH256 | tee onboarding.log
  grep 'FIDO Device Onboard Complete' onboarding.log
}

get_server_logs() {
  [ ! -f "${manufacturer_log}" ] || cat "${manufacturer_log}"
  [ ! -f "${rendezvous_log}" ]   || cat "${rendezvous_log}"
  [ ! -f "${owner_log}" ]        || cat "${owner_log}"
}

run_service () {
  local service=$1
  local port=$2
  nohup go-fdo-server serve "${service}:${port}" --db "/tmp/${service}.db" --db-pass '2=,%95QF<uTLLHt' --debug &> "${service}.log" &
}

run_services () {
  run_service ${manufacturer_dns} ${manufacturer_port}
  run_service ${rendezvous_dns} ${rendezvous_port}
  run_service ${owner_dns} ${owner_port}
}

install_client() {
  git clone https://github.com/fido-device-onboard/go-fdo-client.git /tmp/go-fdo-client
  cd /tmp/go-fdo-client
  go build
  sudo install -D -m 755 go-fdo-client /usr/bin/
  rm -rf /tmp/go-fdo-client
  cd -
}

test_onboarding () {
  setup_hostnames
  run_services
  wait_for_fdo_servers_ready
  set_rendezvous_info
  run_device_initialization
  send_voucher_to_owner
  run_to0
  run_fido_device_onboard
}
