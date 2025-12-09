#! /bin/bash

get_rendezvous_info () {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
       --request GET \
       --header 'Content-Type: text/plain' \
       "${manufacturer_url}/api/v1/rvinfo"
}

set_rendezvous_info () {
  local manufacturer_url=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_port=$4
  local rendezvous_protocol=$5
  local rendezvous_info="[{\"dns\": \"${rendezvous_dns}\", \"device_port\": \"${rendezvous_port}\", \"protocol\": \"${rendezvous_protocol}\", \"ip\": \"${rendezvous_ip}\", \"owner_port\": \"${rendezvous_port}\"}]"
  curl --fail --verbose --silent --insecure \
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
  local rendezvous_protocol=$5
  local rendezvous_info="[{\"dns\": \"${rendezvous_dns}\", \"device_port\": \"${rendezvous_port}\", \"protocol\": \"${rendezvous_protocol}\", \"ip\": \"${rendezvous_ip}\", \"owner_port\": \"${rendezvous_port}\"}]"
  curl --fail --verbose --silent --insecure \
       --request PUT \
       --header 'Content-Type: text/plain' \
       --data-raw "${rendezvous_info}" \
       "${manufacturer_url}/api/v1/rvinfo"
}

get_owner_redirect_info () {
  local owner_url=$1
  curl --fail --verbose --silent --insecure \
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
  local protocol=$5
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent --insecure \
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
  local protocol=$5
  rvto2addr="[{\"ip\": \"${ip}\", \"dns\": \"${dns}\", \"port\": \"${port}\", \"protocol\": \"${protocol}\"}]"
  curl --fail --verbose --silent --insecure \
       --request PUT \
       --header 'Content-Type: text/plain' \
       --data-raw "${rvto2addr}" \
       "${owner_url}/api/v1/owner/redirect"
}

get_ov_from_manufacturer () {
  local manufacturer_url=$1
  local guid=$2
  local output=$3
  curl --fail --verbose --silent --insecure \
    "${manufacturer_url}/api/v1/vouchers/${guid}" -o "${output}"
}

send_ov_to_owner () {
  local owner_url=$1
  local output=$2
  [ -s "${output}" ] || { echo "❌ Voucher file not found or empty: ${output}" >&2; return 1; }
  curl --fail --verbose --silent --insecure \
       --request POST \
       --data-binary "@${output}" \
       "${owner_url}/api/v1/owner/vouchers"
}

resell() {
  local owner_url=$1
  local guid=$2
  local new_owner_pubkey=$3
  local output=$4
  [ -s "${new_owner_pubkey}" ] || { echo "❌ Public key file not found or empty: ${new_owner_pubkey}" >&2; return 1; }
  curl --fail --verbose --silent --insecure "${owner_url}/api/v1/owner/resell/${guid}" --data-binary @"${new_owner_pubkey}" -o "${output}"
}

# JSON API functions for /api/v1/ownerinfo endpoint
get_ownerinfo() {
  local owner_url=$1
  local response
  response=$(curl -s -w "HTTP_STATUS:%{http_code}" "${owner_url}/api/v1/ownerinfo" 2>/dev/null)
  local http_status=$(echo "$response" | grep -o "HTTP_STATUS:[0-9]*" | cut -d: -f2)
  local body=$(echo "$response" | sed 's/HTTP_STATUS:[0-9]*$//')
  
  # Return empty string if not found (404) or any error, otherwise return body
  if [ "$http_status" = "200" ]; then
    echo "$body"
  else
    echo ""
  fi
}

set_ownerinfo() {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  local protocol=$5
  local owner_redirect_json
  owner_redirect_json='[{"dns":"'${dns}'","port":"'${port}'","protocol":"'${protocol}'"}]'
  curl -X POST "${owner_url}/api/v1/ownerinfo" \
    -H "Content-Type: application/json" \
    -d "${owner_redirect_json}" \
    -s
}

update_ownerinfo() {
  local owner_url=$1
  local ip=$2
  local dns=$3
  local port=$4
  local protocol=$5
  local owner_redirect_json
  owner_redirect_json='[{"dns":"'${dns}'","port":"'${port}'","protocol":"'${protocol}'"}]'
  curl -X PUT "${owner_url}/api/v1/ownerinfo" \
    -H "Content-Type: application/json" \
    -d "${owner_redirect_json}" \
    -s
}

