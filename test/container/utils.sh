#! /usr/bin/env bash

set -euo pipefail

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/servers/fdo-onboarding-servers.yaml"

container_user="$(id -u):$(id -g)"
export container_user

get_real_ip() {
  local service_name=$1
  docker inspect --format='{{.NetworkSettings.Networks.fdo.IPAddress}}' "${service_name}"
}

set_hostnames() {
  echo "⭐ Adding hostnames to '/etc/hosts'"
  for service_name in $(docker compose -f ${servers_compose_file} ps -a --format "{{.Name}}"); do
    service_dns="$(docker inspect "${service_name}" --format='{{index .NetworkSettings.Networks.fdo.Aliases 0}}')"
    [ -n "${service_dns}" ] || service_dns="${service_name}"
    set_hostname "$service_dns" "127.0.0.1"
  done
}

unset_hostnames() {
  echo "⭐ Removing hostnames from '/etc/hosts'"
  for service_name in $(docker compose -f ${servers_compose_file} ps -a --format "{{.Name}}"); do
    service_dns="$(docker inspect "${service_name}" --format='{{index .NetworkSettings.Networks.fdo.Aliases 0}}')"
    [ -n "${service_dns}" ] || service_dns="${service_name}"
    unset_hostname "$service_dns" "127.0.0.1"
  done
}

install_client() {
  docker compose --file "${client_compose_file}" build -q
}

uninstall_client() {
  # we don't need to remove any container, all of them are removed after invocation
  return
}

run_go_fdo_client() {
  docker compose --file "${client_compose_file}" run --rm go-fdo-client "$@"
}

install_server() {
  docker compose --file "${servers_compose_file}" build -q
}

uninstall_server() {
  docker compose --file "${servers_compose_file}" down
}

start_service() {
  local service_name=$1
  docker compose --file "${servers_compose_file}" up -d "${service_name}"
}

start_services() {
  docker compose --file "${servers_compose_file}" up -d
  set_hostnames
}

stop_service() {
  local service_name=$1
  docker compose --file "${servers_compose_file}" stop "${service_name}"
}

stop_services() {
  docker compose --file "${servers_compose_file}" stop
}

get_server_logs() {
  for service_name in $(docker compose --file ${servers_compose_file} ps -a --format "{{.Name}}"); do
    docker compose --file "${servers_compose_file}" logs "${service_name}"
  done
}
