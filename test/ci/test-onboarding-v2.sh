#! /usr/bin/env bash

set -euo pipefail

# Source base test script
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-onboarding.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/fdo-api-v2.sh"

# Override rv_info to use V2 format (array of arrays with integer ports)
rv_info="[[{\"dns\":\"${rendezvous_dns}\"},{\"device_port\":${rendezvous_port}},{\"protocol\":\"${rendezvous_protocol}\"},{\"ip\":\"${rendezvous_ip}\"},{\"owner_port\":${rendezvous_port}}]]"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
