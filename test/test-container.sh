#! /bin/bash

set -xeuo pipefail
shopt -s expand_aliases

creds_dir="/tmp/go-fdo/device-credentials"
alias go-fdo-client="docker run --rm --volume '${creds_dir}:${creds_dir}' --network fdo --workdir '${creds_dir}' go-fdo-client"

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"


update_ips() {
  rendezvous_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "rendezvous" | jq -r '.[]|.IPAddress')"
  owner_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "owner" | jq -r '.[]|.IPAddress')"
  new_owner_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "new-owner" | jq -r '.[]|.IPAddress')"
}

get_server_logs() {
  docker logs manufacturer
  docker logs rendezvous
  docker logs owner
  docker logs new-owner
}

run_services () {
  docker compose -f deployments/compose/servers.yaml up -d
}

install_server () {
  docker compose -f deployments/compose/servers.yaml build
}

install_client() {
  docker compose -f deployments/compose/client.yaml build
}
