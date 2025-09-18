#! /bin/bash

set -xeuo pipefail

BASE_DIR=/tmp/go-fdo
UPLOADS_DIR=${BASE_DIR}/uploads
CREDS_DIR=${BASE_DIR}/device-credentials
ONBOARD_LOG=${BASE_DIR}/onboarding-owner.log

cleanup() {
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose -f deployments/compose/servers.yaml -f deployments/compose/fsim-upload.override.yaml down || :
  else
    docker compose -f deployments/compose/servers.yaml -f deployments/compose/fsim-upload.override.yaml down || :
  fi
  # Use Docker to clean up files with proper permissions
  if [ -d "${BASE_DIR}" ]; then
    docker run --rm --volume "${BASE_DIR}:${BASE_DIR}" alpine sh -c "rm -rf ${BASE_DIR}/*" || true
    rmdir "${BASE_DIR}" 2>/dev/null || true
  fi
}

trap cleanup EXIT

# Create directories with proper ownership
mkdir -p "${UPLOADS_DIR}" "${CREDS_DIR}" "${BASE_DIR}/tmp"
# Ensure the current user owns the directories and set proper permissions
chown -R "$(id -u):$(id -g)" "${BASE_DIR}" 2>/dev/null || true
# Set world-writable permissions for the container to access
chmod -R 777 "${BASE_DIR}" 2>/dev/null || true

# Generate certificates needed by the services
echo "Generating certificates..."
bash -c 'source test/test-makefile.sh; generate_certs'

# Choose compose command
docker_compose() {
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
  else
    docker compose "$@"
  fi
}

# Build images
docker_compose -f deployments/compose/networks.yaml -f deployments/compose/servers.yaml build
# Build go-fdo-client from upstream
echo "Building go-fdo-client from upstream..."
docker_compose -f deployments/compose/client.yaml build
# Bring up servers with FSIM upload enabled on owner
docker_compose -f deployments/compose/networks.yaml -f deployments/compose/servers.yaml -f deployments/compose/fsim-upload.override.yaml up -d --scale new-owner=0

# Wait for services to start
echo "Waiting for services to be ready..."
sleep 20

# Check if services are actually running
echo "Checking service status..."
docker logs manufacturer 2>&1 | tail -5
echo "Testing manufacturer connectivity..."
curl -v http://127.0.0.1:8038/health || echo "Manufacturer health check failed"

# Configure RV info using container IPs
bash -c 'source test/test-container.sh; source test/test-makefile.sh; \
  update_ips; \
  curl --fail --verbose --silent \
    --header "Content-Type: text/plain" \
    --request POST \
    --data-raw "[[[5,\"rendezvous\"],[3,8041],[12,1],[2,\"${rendezvous_ip}\"],[4,8041]]]" \
    "http://${manufacturer_ip}:8038/api/v1/rvinfo"'

# File will be created by run_fido_device_onboard function when needed

# Perform full onboarding using container IPs
bash -c "source test/test-container.sh; source test/test-makefile.sh; \
  run_device_initialization; \
  guid=\$(get_device_guid); \
  echo \"Device GUID: \${guid}\"; \
  update_ips; \
  curl --fail --verbose --silent \"http://\${manufacturer_ip}:8038/api/v1/vouchers/\${guid}\" -o \"\${owner_ov}\"; \
  curl --location --request POST \"http://\${owner_ip}:8043/api/v1/owner/redirect\" \
    --header \"Content-Type: text/plain\" \
    --data-raw \"[[\\\"owner\\\",\\\"owner\\\",8043,3]]\"; \
  curl --fail --verbose --silent \"http://\${owner_ip}:8043/api/v1/owner/vouchers\" --data-binary \"@\${owner_ov}\"; \
  curl --fail --verbose --silent \"http://\${owner_ip}:8043/api/v1/to0/\${guid}\"; \
  run_fido_device_onboard \"${ONBOARD_LOG}\" \"upload\""

# Verify FSIM upload occurred by checking for file in upload directory
[ -f "${UPLOADS_DIR}/uploaded.txt" ] || { echo "FSIM upload file not found"; exit 1; }

# Validate integrity: checksum and byte-for-byte compare
SRC_FILE="${CREDS_DIR}/uploaded.txt"
DST_FILE="${UPLOADS_DIR}/uploaded.txt"

# Use Docker to read the uploaded file with proper permissions
echo "Verifying uploaded file integrity..."
SRC_SHA=$(sha256sum "${SRC_FILE}" | awk '{print $1}')
DST_SHA=$(docker run --rm --volume "${UPLOADS_DIR}:${UPLOADS_DIR}" alpine sh -c "sha256sum ${DST_FILE}" | awk '{print $1}')

if [ "${SRC_SHA}" != "${DST_SHA}" ]; then
  echo "Checksum mismatch: src=${SRC_SHA} dst=${DST_SHA}"
  exit 1
fi

# Verify file content using Docker
echo "Verifying file content..."
SRC_CONTENT=$(cat "${SRC_FILE}")
DST_CONTENT=$(docker run --rm --volume "${UPLOADS_DIR}:${UPLOADS_DIR}" alpine sh -c "cat ${DST_FILE}")

if [ "${SRC_CONTENT}" != "${DST_CONTENT}" ]; then
  echo "Content mismatch between source and destination"
  echo "Source: '${SRC_CONTENT}'"
  echo "Destination: '${DST_CONTENT}'"
  exit 1
fi

echo "PASS: FSIM upload verified with checksum match: ${DST_FILE}"
echo "SUCCESS: File content matches - FSIM upload working correctly!"
echo "File content: '${DST_CONTENT}'" 