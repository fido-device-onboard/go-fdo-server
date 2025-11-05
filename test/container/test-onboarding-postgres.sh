#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../ci/test-onboarding.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/server/fdo-onboarding-servers-postgres.yaml"

generate_manufacturer_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "postgres"
  dsn: "host=manufacturer-db user=manufacturer password=Passw0rd dbname=manufacturer port=5432 sslmode=disable TimeZone=Europe/Madrid"
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
  type: "postgres"
  dsn: "host=rendezvous-db user=rendezvous password=Passw0rd dbname=rendezvous port=5432 sslmode=disable TimeZone=Europe/Madrid"
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
  type: "postgres"
  dsn: "host=owner-db user=owner password=Passw0rd dbname=owner port=5432 sslmode=disable TimeZone=Europe/Madrid"
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

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
