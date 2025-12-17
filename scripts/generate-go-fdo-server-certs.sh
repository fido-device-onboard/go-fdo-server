#! /bin/bash

# This script is used to generate a set of self-signed test
# certificates and keys required for running the Go FDO servers. It is
# provided for testing/documentation purposes only.

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

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
