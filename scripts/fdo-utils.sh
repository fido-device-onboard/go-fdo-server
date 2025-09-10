#! /bin/bash

set_rendezvous_info () {
  local manufacturer_service=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_port=$4
  curl --fail --verbose --silent \
       --header 'Content-Type: text/plain' \
       --request POST \
       --data-raw "[[[5,\"${rendezvous_dns}\"],[3,${rendezvous_port}],[12,1],[2,\"${rendezvous_ip}\"],[4,${rendezvous_port}]]]" \
       "http://${manufacturer_service}/api/v1/rvinfo"
}

update_rendezvous_info () {
  local manufacturer_service=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_ip=$4
  curl --fail --verbose --silent \
       --request PUT \
       --header 'Content-Type: text/plain' \
       --data-raw "[[[5,\"${rendezvous_dns}\"],[3,${rendezvous_port}],[12,1],[2,\"${rendezvous_ip}\"],[4,${rendezvous_port}]]]" \
       "http://${manufacturer_service}/api/v1/rvinfo"
}

set_owner_redirect_info () {
  local service=$1
  local ip=$2
  local port=$3
  curl --location --request POST "http://${service}/api/v1/owner/redirect" \
       --header 'Content-Type: text/plain' \
       --data-raw "[[\"${ip}\",\"${ip}\",${port},3]]"
}

get_ov_from_manufacturer () {
  local manufacturer_service=$1
  local guid=$2
  local output=$3
  curl --fail --verbose --silent "http://${manufacturer_service}/api/v1/vouchers/${guid}" -o "${output}"
}

send_ov_to_owner () {
  local owner_service=$1
  local output=$2
  curl --fail --verbose --silent "http://${owner_service}/api/v1/owner/vouchers" --data-binary "@${output}"
}

run_to0 () {
  local owner_service=$1
  local guid=$2
  curl --fail --verbose --silent "http://${owner_service}/api/v1/to0/${guid}"
}

resell() {
  local owner_service=$1
  local guid=$2
  local new_owner_pubkey=$3
  local output=$4
  curl --fail --verbose --silent "http://${owner_service}/api/v1/owner/resell/${guid}" --data-binary @"${new_owner_pubkey}" -o "${output}"
}

