#! /usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/utils.sh"

# V2 API functions (local overrides)
get_rendezvous_info_v2() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request GET \
    "${manufacturer_url}/api/v2/rvinfo"
}

set_rendezvous_info_v2() {
  local manufacturer_url=$1
  local rv_info_v2=$2
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v2}" \
    "${manufacturer_url}/api/v2/rvinfo"
}

update_rendezvous_info_v2() {
  local manufacturer_url=$1
  local rv_info_v2=$2
  curl --fail --verbose --silent --insecure \
    --request PUT \
    --header 'Content-Type: application/json' \
    --data-raw "${rv_info_v2}" \
    "${manufacturer_url}/api/v2/rvinfo"
}

delete_rendezvous_info_v2() {
  local manufacturer_url=$1
  curl --fail --verbose --silent --insecure \
    --request DELETE \
    "${manufacturer_url}/api/v2/rvinfo"
}

run_test() {
  log_info "Setting the error trap handler"
  trap on_failure EXIT

  log_info "Environment variables"
  show_env

  log_info "Creating directories"
  create_directories

  log_info "Build and install 'go-fdo-server' binary"
  install_server

  log_info "Generating service certificates"
  generate_service_certs

  log_info "Configuring manufacturer service"
  configure_services "${manufacturer_service_name}"

  log_info "Configure DNS and start manufacturer service"
  start_services "${manufacturer_service_name}"

  log_info "Wait for manufacturer service to be ready"
  wait_for_services_ready "${manufacturer_service_name}"

  log_info "=== Testing RVInfo API Lifecycle ==="

  # Step 1: Verify empty state (baseline)
  log_info "Step 1: GET /api/v2/rvinfo - Verify empty state"
  response=$(get_rendezvous_info_v2 "${manufacturer_url}")
  log_info "Response: ${response}"

  if [ "${response}" != "[]" ]; then
    log_error "Expected empty array, got: ${response}"
    return 1
  fi
  log_info "✅ Initial state is empty"

  # Step 2: Test validation - invalid data (negative test)
  log_info "Step 2: PUT /api/v2/rvinfo - Test validation (missing DNS/IP)"
  invalid_rvinfo='[[
    {"protocol": "http"},
    {"owner_port": 8080}
  ]]'

  if update_rendezvous_info_v2 "${manufacturer_url}" "${invalid_rvinfo}" 2>/dev/null; then
    log_error "Invalid data should have been rejected"
    return 1
  fi
  log_info "✅ Invalid data correctly rejected (missing DNS/IP)"

  # Step 3: Create RVInfo configuration
  log_info "Step 3: PUT /api/v2/rvinfo - Create initial configuration"
  initial_rvinfo='[[
    {"dns": "rv.example.com"},
    {"protocol": "https"},
    {"owner_port": 8443}
  ]]'

  response=$(update_rendezvous_info_v2 "${manufacturer_url}" "${initial_rvinfo}")
  log_info "Response: ${response}"

  if ! echo "${response}" | jq -e '.[0] | type == "array"' >/dev/null; then
    log_error "Response is not an array of arrays"
    return 1
  fi
  log_info "✅ RVInfo created successfully"

  # Step 4: Verify creation
  log_info "Step 4: GET /api/v2/rvinfo - Verify creation"
  response=$(get_rendezvous_info_v2 "${manufacturer_url}")
  log_info "Response: ${response}"

  directive_count=$(echo "${response}" | jq '. | length')
  if [ "${directive_count}" != "1" ]; then
    log_error "Expected 1 directive, got ${directive_count}"
    return 1
  fi

  if ! echo "${response}" | jq -e '.[0][0] | has("dns")' >/dev/null; then
    log_error "First instruction doesn't have 'dns' key"
    return 1
  fi
  log_info "✅ Configuration verified (1 directive with DNS)"

  # Step 5: Update with RV bypass configuration
  log_info "Step 5: PUT /api/v2/rvinfo - Update with RV bypass"
  bypass_rvinfo='[[
    {"dns": "owner.example.com"},
    {"protocol": "https"},
    {"owner_port": 8443},
    {"rv_bypass": true}
  ]]'

  response=$(update_rendezvous_info_v2 "${manufacturer_url}" "${bypass_rvinfo}")
  log_info "Response: ${response}"
  log_info "✅ RVInfo updated with RV bypass"

  # Step 6: Verify RV bypass configuration
  log_info "Step 6: GET /api/v2/rvinfo - Verify RV bypass"
  response=$(get_rendezvous_info_v2 "${manufacturer_url}")

  if ! echo "${response}" | jq -e '.[0][3] | has("rv_bypass")' >/dev/null; then
    log_error "Fourth instruction doesn't have 'rv_bypass' key"
    return 1
  fi

  bypass_value=$(echo "${response}" | jq -r '.[0][3].rv_bypass')
  if [ "${bypass_value}" != "true" ]; then
    log_error "Expected rv_bypass to be true, got ${bypass_value}"
    return 1
  fi
  log_info "✅ RV bypass configuration verified"

  # Step 7: Update with multi-directive configuration
  log_info "Step 7: PUT /api/v2/rvinfo - Update with multiple directives"
  multi_rvinfo='[
    [
      {"dns": "rv-primary.example.com"},
      {"protocol": "https"},
      {"owner_port": 8443}
    ],
    [
      {"ip": "192.168.1.100"},
      {"protocol": "http"},
      {"owner_port": 8080}
    ]
  ]'

  response=$(update_rendezvous_info_v2 "${manufacturer_url}" "${multi_rvinfo}")
  log_info "Response: ${response}"
  log_info "✅ RVInfo updated with multiple directives"

  # Step 8: Verify multi-directive configuration
  log_info "Step 8: GET /api/v2/rvinfo - Verify multiple directives"
  response=$(get_rendezvous_info_v2 "${manufacturer_url}")

  directive_count=$(echo "${response}" | jq '. | length')
  if [ "${directive_count}" != "2" ]; then
    log_error "Expected 2 directives, got ${directive_count}"
    return 1
  fi

  # Verify first directive has DNS
  if ! echo "${response}" | jq -e '.[0][0] | has("dns")' >/dev/null; then
    log_error "First directive should have DNS"
    return 1
  fi

  # Verify second directive has IP
  if ! echo "${response}" | jq -e '.[1][0] | has("ip")' >/dev/null; then
    log_error "Second directive should have IP"
    return 1
  fi
  log_info "✅ Multiple directives verified (DNS + IP fallback)"

  # Step 9: Delete RVInfo configuration
  log_info "Step 9: DELETE /api/v2/rvinfo - Delete configuration"
  response=$(delete_rendezvous_info_v2 "${manufacturer_url}")
  log_info "Response (deleted config): ${response}"

  # Verify response contains the deleted configuration
  if ! echo "${response}" | jq -e 'type == "array"' >/dev/null; then
    log_error "DELETE should return the deleted configuration"
    return 1
  fi
  log_info "✅ RVInfo deleted, returned deleted configuration"

  # Step 10: Verify deletion (back to empty state)
  log_info "Step 10: GET /api/v2/rvinfo - Verify deletion"
  response=$(get_rendezvous_info_v2 "${manufacturer_url}")
  log_info "Response after deletion: ${response}"

  if [ "${response}" != "[]" ]; then
    log_error "RVInfo should be empty after deletion, but got: ${response}"
    return 1
  fi
  log_info "✅ Configuration deleted, state is empty"

  log_info "=== All RVInfo API lifecycle tests passed! ==="

  log_info "Unsetting the error trap handler"
  trap - EXIT
  test_pass
}

# Allow running directly
[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  run_test
  cleanup
}
