#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

cert_dir="/etc/pki/go-fdo-server"
subj="/C=US/O=FDO/CN=Owner"
key="${cert_dir}/owner.key"
crt="${cert_dir}/owner.crt"

generate_cert "${key}" "${crt}" "${subj}"
