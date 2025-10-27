#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../ci/test-onboarding.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

client_compose_file="deployments/compose/client/fdo-client.yaml"
servers_compose_file="deployments/compose/server/fdo-onboarding-servers-postgres.yaml"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
