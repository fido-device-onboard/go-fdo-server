#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../ci/utils.sh"

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/server/fdo-onboarding-servers.yaml"

# Export base_dir explicitly for Docker Compose
export base_dir

# Export container_user explicitly for Docker Compose
container_user="$(id -u):$(id -g)"
export container_user

# Container working directory (default to /workdir if not set)
container_working_dir="${container_working_dir:-/workdir}"
export container_working_dir

get_real_ip() {
  local service_name=$1
  docker inspect --format='{{.NetworkSettings.Networks.fdo.IPAddress}}' "${service_name}"
}

set_hostnames() {
  for service_name in $(docker compose -f ${servers_compose_file} config --services | grep -v db); do
    service_dns="$(docker inspect "${service_name}" --format='{{index .NetworkSettings.Networks.fdo.Aliases 0}}')"
    [ -n "${service_dns}" ] || service_dns="${service_name}"
    set_hostname "$service_dns" "127.0.0.1"
  done
}

unset_hostnames() {
  log_info "Removing hostnames from '/etc/hosts'"
  for service_name in $(docker compose -f ${servers_compose_file} config --services "{{.Name}}" | grep -v db); do
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
  # Translate host paths to container paths in arguments
  local args=()
  for arg in "$@"; do
    # Replace base_dir with container_working_dir in paths
    args+=("${arg//$base_dir/$container_working_dir}")
  done
  docker compose --file "${client_compose_file}" run --rm go-fdo-client "${args[@]}"
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
  log_info "Starting services"
  docker compose --file "${servers_compose_file}" up -d
  # We need to add the hosts IPs after starting the services
  # so the resolution within the containers is not affected
  log_info "Adding hostnames to '/etc/hosts'"
  set_hostnames
}

stop_service() {
  local service_name=$1
  docker compose --file "${servers_compose_file}" stop "${service_name}"
}

stop_services() {
  docker compose --file "${servers_compose_file}" stop
}

get_service_logs() {
  local service=$1
  docker compose --file "${servers_compose_file}" logs --no-log-prefix "${service}"
}

get_logs() {
  log_info "Retrieving logs"
  for service in $(docker compose --file ${servers_compose_file} config --services); do
    log "🛑 '${service}' logs:\n"
    get_service_logs ${service}
  done
}

save_service_logs() {
  local service=$1
  local log_file="${logs_dir}/${service}.log"
  get_service_logs "${service}" >"${log_file}"
}

save_logs() {
  log_info "Saving logs"
  for service in $(docker compose --file ${servers_compose_file} config --services); do
    log "\t⚙ Saving '${service}' logs "
    save_service_logs ${service}
    log_success
  done
}

on_failure() {
  trap - ERR
  save_logs
  stop_services
  test_fail
}
