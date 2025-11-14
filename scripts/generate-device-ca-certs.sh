#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

cert_dir="/etc/pki/go-fdo-server"
subj="/C=US/O=FDO/CN=Device CA"
key="${cert_dir}/device-ca.key"
crt="${cert_dir}/device-ca.crt"
pub="${cert_dir}/device-ca.pub"

generate_cert "${key}" "${crt}" "${subj}"
chmod g+r "${key}"
