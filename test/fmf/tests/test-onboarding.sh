#!/bin/bash

set -euo pipefail

# Parse command line arguments
export USE_TLS=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --tls)
      USE_TLS=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Source the common test logic
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../../ci/test-onboarding.sh"

# Source the FMF-specific utils (will use USE_TLS variable)
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/utils.sh"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
