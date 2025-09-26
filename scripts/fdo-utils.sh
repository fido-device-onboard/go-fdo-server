#! /bin/bash

get_rendezvous_info () {
  local manufacturer_url=$1
  curl --fail --verbose --silent \
       --request GET \
       --header 'Content-Type: text/plain' \
       "${manufacturer_url}/api/v1/rvinfo"
}

set_rendezvous_info () {
  local manufacturer_url=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_port=$4
  local rendezvous_info="[{\"dns\": \"${rendezvous_dns}\", \"device_port\": \"${rendezvous_port}\", \"protocol\": \"http\", \"ip\": \"${rendezvous_ip}\", \"owner_port\": \"${rendezvous_port}\"}]"
  curl --fail --verbose --silent \
       --request POST \
       --header 'Content-Type: text/plain' \
       --data-raw "${rendezvous_info}" \
       "${manufacturer_url}/api/v1/rvinfo"
}

update_rendezvous_info () {
  local manufacturer_url=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_port=$4
  local rendezvous_info="[{\"dns\": \"${rendezvous_dns}\", \"device_port\": \"${rendezvous_port}\", \"protocol\": \"http\", \"ip\": \"${rendezvous_ip}\", \"owner_port\": \"${rendezvous_port}\"}]"
  curl --fail --verbose --silent \
       --request PUT \
       --header 'Content-Type: text/plain' \
       --data-raw "${rendezvous_info}" \
       "${manufacturer_url}/api/v1/rvinfo"
}

get_owner_redirect_info () {
  local owner_url=$1
  curl --fail --verbose --silent \
       --header 'Content-Type: text/plain' \
       "${owner_url}/api/v1/owner/redirect"
}

set_owner_redirect_info () {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  # TransportProtocol /= (
  #     ProtTCP:    1,     ;; bare TCP stream
  #     ProtTLS:    2,     ;; bare TLS stream
  #     ProtHTTP:   3,
  #     ProtCoAP:   4,
  #     ProtHTTPS:  5,
  #     ProtCoAPS:  6,
  # )
  local protocol=${5:-http}
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent \
       --request POST \
       --header 'Content-Type: text/plain' \
       --data-raw "${rvto2addr}" \
       "${owner_url}/api/v1/owner/redirect"
}

update_owner_redirect_info () {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  # TransportProtocol /= (
  #     ProtTCP:    1,     ;; bare TCP stream
  #     ProtTLS:    2,     ;; bare TLS stream
  #     ProtHTTP:   3,
  #     ProtCoAP:   4,
  #     ProtHTTPS:  5,
  #     ProtCoAPS:  6,
  # )
  local protocol=${5:-http}
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent \
       --request PUT \
       --header 'Content-Type: text/plain' \
       --data-raw "${rvto2addr}" \
       "${owner_url}/api/v1/owner/redirect"
}

get_ov_from_manufacturer () {
  local manufacturer_url=$1
  local guid=$2
  local output=$3
  curl --fail --verbose --silent \
    "${manufacturer_url}/api/v1/vouchers/${guid}" -o "${output}"
}

send_ov_to_owner () {
  local owner_url=$1
  local output=$2
  curl --fail --verbose --silent \
       --request POST \
       --data-binary "@${output}" \
       "${owner_url}/api/v1/owner/vouchers"
}

run_to0 () {
  local owner_url=$1
  local guid=$2
  curl --fail --verbose --silent "${owner_url}/api/v1/to0/${guid}"
}

resell() {
  local owner_url=$1
  local guid=$2
  local new_owner_pubkey=$3
  local output=$4
  curl --fail --verbose --silent "${owner_url}/api/v1/owner/resell/${guid}" --data-binary @"${new_owner_pubkey}" -o "${output}"
}

