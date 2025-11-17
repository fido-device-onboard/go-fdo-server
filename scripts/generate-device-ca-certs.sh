#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

cert_dir="/etc/pki/go-fdo-server"
subj="/C=US/O=FDO/CN=Device CA"
key="${cert_dir}/device-ca-example.key"
crt="${cert_dir}/device-ca-example.crt"

generate_cert "${key}" "${crt}" "${subj}"
chmod g+r "${key}"
