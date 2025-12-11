#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

cert_dir="/etc/pki/go-fdo-server"
subj="/C=US/O=FDO/CN=Manufacturer"
key="${cert_dir}/manufacturer-example.key"
crt="${cert_dir}/manufacturer-example.crt"

# Do not overwrite existing cert/key files unless one of the pair is
# missing
if [[ ! -f "${key}" || ! -f "${crt}" ]]; then
  rm -f "${key}" "${crt}"
fi
generate_cert "${key}" "${crt}" "${subj}"
chmod g+r "${key}"
