#!/bin/bash

set -xeuo pipefail

# Source the existing test framework
source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/test-makefile.sh"

# Wget-specific configuration
wget_test_dir="${base_dir}/tests/wget-fsim"
wget_httpd_dir="${wget_test_dir}/httpd"
wget_download_dir="${wget_test_dir}/download"
wget_http_port=8888
wget_http_pid=""
wget_owner_pid=""

# Ensure the http server process is stopped
trap 'wget_stop_http_server' EXIT

# setup directories used by wget test
wget_setup_directories() {
  mkdir -p "${wget_httpd_dir}"
  mkdir -p "${wget_download_dir}"
  echo "Created directories:"
  echo "  HTTP server directory: ${wget_httpd_dir}"
  echo "  Download directory: ${wget_download_dir}"
}

# Create a file for downloading via wget
wget_create_test_file() {
  local pathname=$1
  dd if=/dev/urandom of="${pathname}" bs=1M count=2 2>/dev/null
  echo "Created test file: ${pathname} ($(stat -c%s "${pathname}") bytes)"
}

wget_start_http_server() {
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

wget_stop_http_server() {
  if [ -n "${wget_http_pid}" ] && kill -0 ${wget_http_pid} 2>/dev/null; then
    kill ${wget_http_pid} 2>/dev/null || true
    wait ${wget_http_pid} 2>/dev/null || true
    echo "HTTP server stopped"
  fi
  wget_http_pid=""
}

# Start the manufacturing and rendezvous servers. Each testcase will
# start the owner server
wget_start_mfg_rv_servers() {
  run_service manufacturing ${manufacturer_service} manufacturer ${manufacturer_log} \
    --manufacturing-key="${manufacturer_key}" \
    --owner-cert="${owner_crt}" \
    --device-ca-cert="${device_ca_crt}" \
    --device-ca-key="${device_ca_key}"
  run_service rendezvous ${rendezvous_service} rendezvous ${rendezvous_log}
  wait_for_service "${manufacturer_service}"
  wait_for_service "${rendezvous_service}"
  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${manufacturer_ip} ${rendezvous_port}
  echo "Manufacturing and Rendezvous servers running"
}

# Start the owner server with per-test configuration
wget_start_owner_server() {
  run_service owner ${owner_service} owner ${owner_log} \
              --owner-key="${owner_key}" \
              --device-ca-cert="${device_ca_crt}" \
              $@
  wget_owner_pid=$!
  wait_for_service "${owner_service}"
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  echo "Owner service running"
}

# Stop the owner server, leaving its database intact so it can be
# restarted
wget_stop_owner_server() {
  if [ -n "${wget_owner_pid}" ] && kill -0 ${wget_owner_pid} 2>/dev/null; then
    kill ${wget_owner_pid} 2>/dev/null || true
    wait ${wget_owner_pid} 2>/dev/null || true
  fi
  wget_owner_pid=""
}

# teardown servers and cleanup state so they can be restarted by each
# test
wget_cleanup_servers() {
  stop_services
  cleanup_service manufacturer
  cleanup_service owner
  cleanup_service rendezvous
  wget_owner_pid=""
}

# verify that downloaded file is the same as the source
wget_compare_files() {
  local src_file=$1
  local dst_file=$2

  # Check if file was downloaded
  if [ ! -f "${dst_file}" ]; then
    echo "ERROR: Downloaded file not found at ${dst_file}"
    return 1
  fi

  echo "Downloaded file found: ${dst_file}"
  echo "File size: $(stat -c%s "${dst_file}") bytes"

  # Compare file contents using md5sum
  local original_hash=$(md5sum "${src_file}" | cut -d' ' -f1)
  local downloaded_hash=$(md5sum "${dst_file}" | cut -d' ' -f1)

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

# Cleanup the test environment. This function must be idenpotent.
wget_cleanup() {
  echo "======================== Cleaning up wget FSIM test ================================"
  wget_stop_http_server
  cleanup
}

# Verify that wget can transfer two files successfully
test_wget_simple() {
  echo "======================== Running ${FUNCNAME[0]} ============================="
  wget_create_test_file "${wget_httpd_dir}/test_file1.bin"
  wget_create_test_file "${wget_httpd_dir}/test_file2.bin"
  wget_start_mfg_rv_servers
  wget_start_owner_server --command-wget "http://localhost:${wget_http_port}/test_file1.bin" \
                          --command-wget "http://localhost:${wget_http_port}/test_file2.bin"

  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"

  # Verify the downloaded file is exactly the same as the source file
  wget_compare_files "${wget_httpd_dir}/test_file1.bin" "${wget_download_dir}/test_file1.bin"
  wget_compare_files "${wget_httpd_dir}/test_file2.bin" "${wget_download_dir}/test_file2.bin"

  rm -f "${wget_httpd_dir}/test_file1.bin" "${wget_download_dir}/test_file1.bin" \
     "${wget_httpd_dir}/test_file2.bin" "${wget_download_dir}/test_file2.bin"
  wget_cleanup_servers
  echo "======================== Test ${FUNCNAME[0]} Pass! ============================="
}


# Verify that wget will overwrite an existing destination file
test_wget_overwrite() {
  echo "======================== Running ${FUNCNAME[0]} ============================="
  wget_create_test_file "${wget_httpd_dir}/test_file.bin"
  echo "Overwrite me!" > "${wget_download_dir}/test_file.bin"
  wget_start_mfg_rv_servers
  wget_start_owner_server --command-wget "http://localhost:${wget_http_port}/test_file.bin"

  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"

  # Verify the downloaded file has overwritten the existing file
  wget_compare_files "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin"

  rm -f "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin" 
  wget_cleanup_servers
  echo "======================== Test ${FUNCNAME[0]} Pass! ============================="
}


# This test verifies that the client handles a bad wget command
# gracefully: in particular the client's credential blob is not
# corrupted so device initialization can be re-tried.
test_wget_bad_url() {
  echo "======================== Running ${FUNCNAME[0]} ============================="
  local -  # allow this function to disable exit on failure
  wget_start_mfg_rv_servers
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})

  # start an owner with a bad HTTP address. This simulates the case
  # where the server is not available during device onboard.
  wget_start_owner_server --command-wget "http://notthere.invalid:${wget_http_port}/test_file.bin"
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"

  set +e  # disable exit of failure, we expect onboard to fail here
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"
  if [ "$?" == "0" ]; then
    echo "Expected device onboard to fail!"
    exit 1
  fi
  set -e

  if ! grep -q "no such host" ${owner_onboard_log} ; then
    echo "Expected client to report that the URL was wrong"
    exit 1
  fi

  # stop the owner server and re-start it with a good URL. The owner
  # database was preserved so the voucher is present in the database.
  # Expect onboarding to succeeda
  wget_stop_owner_server
  wget_create_test_file "${wget_httpd_dir}/test_file.bin"
  wget_start_owner_server --command-wget "http://localhost:${wget_http_port}/test_file.bin"  
  run_to0 ${owner_service} "${guid}"
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"
  wget_compare_files "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin"
  rm -f "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin" 
  wget_cleanup_servers
  echo "======================== Test ${FUNCNAME[0]} Pass! ============================="
}


# This test verifies that the client handles a failure to download the
# wget target file gracefully: in particular the client's credential
# blob is not corrupted so device initialization can be re-tried once
# the file is present.
test_wget_missing_file() {
  echo "======================== Running ${FUNCNAME[0]} ============================="
  local -  # allow this function to disable exit on failure
  wget_start_mfg_rv_servers
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})

  # test_file.bin has not yet been created
  wget_start_owner_server --command-wget "http://localhost:${wget_http_port}/test_file.bin"
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  send_ov_to_owner ${owner_service} ${owner_ov}
  run_to0 ${owner_service} "${guid}"

  set +e  # disable exit of failure, we expect onboard to fail here
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"
  if [ "$?" == "0" ]; then
    echo "Expected device onboard to fail!"
    exit 1
  fi
  set -e

  if ! grep -q "expected status 200, got 404" ${owner_onboard_log}; then
    echo "Expected client to report 404 not found error"
    exit 1
  fi

  # now create the file and re-onboard: expect success
  wget_create_test_file "${wget_httpd_dir}/test_file.bin"
  run_fido_device_onboard ${owner_onboard_log} --wget-dir "${wget_download_dir}"
  wget_compare_files "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin"
  rm -f "${wget_httpd_dir}/test_file.bin" "${wget_download_dir}/test_file.bin" 
  wget_cleanup_servers
  echo "======================== Test ${FUNCNAME[0]} Pass! ============================="
}


# Main test function
run_wget_fsim_tests() {
  echo "======================== Starting wget FSIM test ==================================="
  echo "======================== Make sure the env is clean ========================================="
  wget_cleanup
  echo "======================== Generating service certificates ===================================="
  generate_certs
  echo "======================== Install 'go-fdo-client' binary ====================================="
  install_client
  echo "======================== Install 'go-fdo-server' binary ====================================="
  install_server
  echo "======================== Configure the environment  ========================================="
  wget_setup_directories
  wget_start_http_server
  setup_hostnames
  update_ips
  echo "======================== Running wget FSM testcases ============================="
  test_wget_simple
  test_wget_overwrite
  test_wget_bad_url
  test_wget_missing_file
  echo "======================== Clean the environment =============================================="
  wget_cleanup
  echo "======================== wget FSIM test completed successfully ============================="
}
