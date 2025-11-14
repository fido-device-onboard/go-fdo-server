#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

cert_dir="/etc/pki/go-fdo-server"
subj="/C=US/O=FDO/CN=Manufacturer"
key="${cert_dir}/manufacturer.key"
crt="${cert_dir}/manufacturer.crt"

generate_cert "${key}" "${crt}" "${subj}"
chmod g+r "${key}"
