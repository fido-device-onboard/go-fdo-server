#!/bin/bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../../ci/test-onboarding.sh"
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/utils.sh"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
