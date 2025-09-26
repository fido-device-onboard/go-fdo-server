#! /bin/bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../ci/test-fsim-download.sh"
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/utils.sh"

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/server/fsim-fdo-download-servers.yaml"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test

