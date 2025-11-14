#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../ci/test-onboarding-config.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/server/fdo-onboarding-servers-config.yaml"

directories+=("${configs_dir}")

# For containers we need to modify all file paths in the generated configuration
# files to be based on container_working_dir
configure_services() {
  generate_https_certs
  echo "⭐ Configuring services"
  for service in "${services[@]}"; do
    configure_service "${service}"
    local conf_file="${service}_config_file"
    sed -i "s%${base_dir}%${container_working_dir}%g" "${!conf_file}"
  done
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || { run_test && cleanup; }
