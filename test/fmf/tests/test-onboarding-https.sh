#!/bin/bash

set -euo pipefail

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-onboarding.sh"

# Force all services to use HTTPS for this test
manufacturer_protocol=https
manufacturer_url="${manufacturer_protocol}://${manufacturer_service}"
manufacturer_health_url="${manufacturer_url}/health"
rendezvous_protocol=https
rendezvous_url="${rendezvous_protocol}://${rendezvous_service}"
rendezvous_health_url="${rendezvous_url}/health"
owner_protocol=https
owner_url="${owner_protocol}://${owner_service}"
owner_health_url="${owner_url}/health"

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test


