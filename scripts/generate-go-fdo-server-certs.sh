#! /bin/bash

# This script is used to generate a set of self-signed test
# certificates and keys required for running the Go FDO servers. It is
# provided for testing/documentation purposes only.

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/cert-utils.sh"
# Ensure sudo/root privileges
if [ "$EUID" -ne 0 ]; then
  echo "Error: This script must be run as root." >&2
  echo "" >&2
  echo "Please run with sudo:" >&2
  echo "  sudo $0" >&2
  exit 1
fi

# Check if all required server packages are installed
missing_packages=()

if ! rpm -q go-fdo-server-manufacturer &>/dev/null; then
  missing_packages+=("go-fdo-server-manufacturer")
fi

if ! rpm -q go-fdo-server-rendezvous &>/dev/null; then
  missing_packages+=("go-fdo-server-rendezvous")
fi

if ! rpm -q go-fdo-server-owner &>/dev/null; then
  missing_packages+=("go-fdo-server-owner")
fi

if [ ${#missing_packages[@]} -ne 0 ]; then
  echo "Error: This script requires all FDO server packages to be installed." >&2
  echo "" >&2
  echo "Missing packages: ${missing_packages[*]}" >&2
  echo "" >&2
  echo "Install them with:" >&2
  echo "  sudo dnf install ${missing_packages[*]}" >&2
  echo "" >&2
  echo "This script is intended for single-host testing where all FDO services" >&2
  echo "run on the same machine. For production multi-host deployments, see" >&2
  echo "CERTIFICATE_SETUP.md" >&2
  exit 1
fi

cert_dir="/etc/pki/go-fdo-server"

device_subj="/C=US/O=FDO/CN=Device CA"
device_key="${cert_dir}/device-ca-example.key"
device_crt="${cert_dir}/device-ca-example.crt"

manufacturer_subj="/C=US/O=FDO/CN=Manufacturer"
manufacturer_key="${cert_dir}/manufacturer-example.key"
manufacturer_crt="${cert_dir}/manufacturer-example.crt"

owner_subj="/C=US/O=FDO/CN=Owner"
owner_key="${cert_dir}/owner-example.key"
owner_crt="${cert_dir}/owner-example.crt"

generate_example_cert() {
  key=$1
  crt=$2
  subj=$3
  # generate_cert will do nothing if the key or certificate file is
  # present in order to prevent overwriting existing credentials.
  if [[ ! -f "${key}" || ! -f "${crt}" ]]; then
    # If either file is missing we need to re-generate a new pair
    rm -f "${key}" "${crt}"
  fi
  generate_cert "${key}" "${crt}" "${subj}"
}

# Set the ownership of the device CA credentials to the manufacturer
# server user since it is the only server that needs to use the
# private key for signing.
generate_example_cert "${device_key}" "${device_crt}" "${device_subj}"
chown go-fdo-server-manufacturer:go-fdo-server "${device_key}" "${device_crt}"

# The manufacturer private key must belong to and it must be readable by
# the manufacturer user only as it is the only server using it for
# signing.
generate_example_cert "${manufacturer_key}" "${manufacturer_crt}" "${manufacturer_subj}"
chown go-fdo-server-manufacturer:go-fdo-server "${manufacturer_key}" "${manufacturer_crt}"

# The owner private key must belong to and it must be readable by the
# owner user only as it is the only server using it for signing.
generate_example_cert "${owner_key}" "${owner_crt}" "${owner_subj}"
chown go-fdo-server-owner:go-fdo-server "${owner_key}" "${owner_crt}"

# Display created files
echo ""
echo "Successfully generated FDO server certificates:"
echo ""
echo "Device CA (shared between manufacturer and owner):"
echo "  ${device_key}"
echo "  ${device_crt}"
echo ""
echo "Manufacturer (local to manufacturer server):"
echo "  ${manufacturer_key}"
echo "  ${manufacturer_crt}"
echo ""
echo "Owner (private key for owner server, certificate for owner and manufacturer):"
echo "  ${owner_key}"
echo "  ${owner_crt}"
echo ""
echo "You can now start the FDO services:"
echo "  sudo systemctl start go-fdo-server-manufacturer.service"
echo "  sudo systemctl start go-fdo-server-rendezvous.service"
echo "  sudo systemctl start go-fdo-server-owner.service"
echo ""
