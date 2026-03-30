#! /usr/bin/env bash
# RV bypass test: Device skips TO1 by getting Owner address directly from voucher (TO0 not needed)

set -euo pipefail

# Source base test script
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/test-rv-bypass.sh"
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../scripts/fdo-api-v2.sh"

# Override rv_info to use V2 format with RV bypass (array of arrays with integer ports)
rv_info="[[{\"dns\":\"${owner_dns}\"},{\"device_port\":${owner_port}},{\"protocol\":\"${owner_protocol}\"},{\"ip\":\"${owner_ip}\"},{\"owner_port\":${owner_port}},{\"rv_bypass\":true}]]"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
