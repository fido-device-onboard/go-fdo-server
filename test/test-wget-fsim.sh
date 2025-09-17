#!/bin/bash

set -xeuo pipefail

# Source the existing test framework
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"

# Wget-specific configuration
wget_test_dir="${base_dir}/tests/wget-fsim"
wget_httpd_dir="${wget_test_dir}/httpd"
wget_download_dir="${wget_test_dir}/download"
wget_test_file="${wget_httpd_dir}/test-data.bin"
wget_http_port=8888
wget_http_pid=""

# Set up trap to ensure HTTP server is cleaned up on exit
trap 'stop_http_server' EXIT

# setup directories used by wget test
setup_wget_directories() {
  mkdir -p "${wget_httpd_dir}"
  mkdir -p "${wget_download_dir}"
  echo "Created directories:"
  echo "  HTTP server directory: ${wget_httpd_dir}"
  echo "  Download directory: ${wget_download_dir}"
}

create_test_file() {
  # Create a 2MB file with random data
  dd if=/dev/urandom of="${wget_test_file}" bs=1M count=2 2>/dev/null
  echo "Created test file: ${wget_test_file} ($(stat -c%s "${wget_test_file}") bytes)"
}

start_http_server() {
  cd "${wget_httpd_dir}"
  # Start Python HTTP server in background
  python3 -m http.server ${wget_http_port} > "${base_dir}/http-server.log" 2>&1 &
  wget_http_pid=$!
  cd - > /dev/null

  # Wait a moment for server to start
  sleep 2

  # Verify server is running
  if ! kill -0 ${wget_http_pid} 2>/dev/null; then
    echo "ERROR: Failed to start HTTP server"
    cat "${base_dir}/http-server.log"
    exit 1
  fi

  # Test that server is responding
  local retry=0
  local max_retries=5
  while [ $retry -lt $max_retries ]; do
    if curl -s "http://localhost:${wget_http_port}/test-data.bin" > /dev/null 2>&1; then
      echo "HTTP server started successfully on port ${wget_http_port}"
      return 0
    fi
    echo "Waiting for HTTP server to be ready... (attempt $((retry + 1))/$max_retries)"
    sleep 1
    ((retry++))
  done

  echo "ERROR: HTTP server failed to respond after ${max_retries} attempts"
  cat "${base_dir}/http-server.log"
  exit 1
}

stop_http_server() {
  if [ -n "${wget_http_pid}" ] && kill -0 ${wget_http_pid} 2>/dev/null; then
    kill ${wget_http_pid} 2>/dev/null || true
    wait ${wget_http_pid} 2>/dev/null || true
    echo "HTTP server stopped"
  fi
  wget_http_pid=""
}

# Modified run_services function that adds wget support for owner service
run_services_wget() {
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}
  run_service owner ${owner_service} owner ${owner_log} \
    --owner-key="${owner_key}" \
    --device-ca-cert="${device_ca_crt}" \
    --command-wget "http://localhost:${wget_http_port}/test-data.bin"
}

# Function to verify the wget download
verify_wget_download() {
  local downloaded_file="${wget_download_dir}/test-data.bin"

  # Check if file was downloaded
  if [ ! -f "${downloaded_file}" ]; then
    echo "ERROR: Downloaded file not found at ${downloaded_file}"
    return 1
  fi

  echo "Downloaded file found: ${downloaded_file}"
  echo "File size: $(stat -c%s "${downloaded_file}") bytes"

  # Compare file contents using md5sum
  local original_hash=$(md5sum "${wget_test_file}" | cut -d' ' -f1)
  local downloaded_hash=$(md5sum "${downloaded_file}" | cut -d' ' -f1)

  echo "Original file hash:  ${original_hash}"
  echo "Downloaded file hash: ${downloaded_hash}"

  if [ "${original_hash}" = "${downloaded_hash}" ]; then
    echo "SUCCESS: Downloaded file matches original file"
    return 0
  else
    echo "ERROR: Downloaded file does not match original file"
    return 1
  fi
}

# Setup wget test
setup_wget() {
  echo "======================== Set up wget FSIM test ================================"
  setup_wget_directories
  create_test_file
  start_http_server
}

# Cleanup wget test
cleanup_wget() {
  echo "======================== Cleaning up wget FSIM test ================================"
  stop_http_server
  cleanup
}

# Custom wait function that only checks for one owner service
wait_for_wget_servers_ready() {
  # manufacturer server
  wait_for_service "${manufacturer_service}"
  # Rendezvous server
  wait_for_service "${rendezvous_service}"
  # Owner server
  wait_for_service "${owner_service}"
}

# Modified setup_env function that includes wget setup
setup_env_wget() {
  mkdir -p ${base_dir}
  run_services_wget
  setup_hostnames
  wait_for_wget_servers_ready
  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${manufacturer_ip} ${rendezvous_port}
}

# Modified test_onboarding function that includes wget verification
test_onboarding_wget() {
  update_ips
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"

  # Verify the wget download
  verify_wget_download
}

# Main test function
test_wget_fsim() {
  echo "======================== Starting wget FSIM test ==================================="
  echo "======================== Make sure the env is clean ========================================="
  cleanup_wget
  echo "======================== Setting up wget FSIM test environment ============================="
  setup_wget
  echo "======================== Generating service certificates ===================================="
  generate_certs
  echo "======================== Install 'go-fdo-client' binary ====================================="
  install_client
  echo "======================== Install 'go-fdo-server' binary ====================================="
  install_server
  echo "======================== Configure the environment  ========================================="
  setup_env_wget
  echo "======================== Testing FDO Onboarding with wget FSIM ============================="
  test_onboarding_wget
  echo "======================== Clean the environment =============================================="
  cleanup_wget
  echo "======================== wget FSIM test completed successfully ============================="
}
