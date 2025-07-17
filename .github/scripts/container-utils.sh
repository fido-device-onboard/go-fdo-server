#! /bin/bash

set -xeuo pipefail
shopt -s expand_aliases

alias go-fdo-client="docker run --rm --volume /tmp/device-credentials:/tmp/device-credentials --network fdo --workdir /tmp/device-credentials go-fdo-client"

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/fdo-utils.sh"

rendezvous_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "rendezvous" | jq -r '.[]|.IPAddress')"

get_server_logs() {
  docker logs manufacturer
  docker logs rendezvous
  docker logs owner
}

run_services () {
  return
}

install_client() {
  return
}
