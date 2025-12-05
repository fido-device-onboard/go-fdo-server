#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

ENV_FILE="/etc/sysconfig/go-fdo-server-manufacturer"
[ ! -f "${ENV_FILE}" ] || source "${ENV_FILE}"

conf_dir="${MANUFACTURER_CONF_DIR:-/etc/go-fdo-server}"

subj="${MANUFACTURER_SUBJECT:-/C=US/O=FDO/CN=Manufacturer}"
key="${MANUFACTURER_KEY:-${conf_dir}/manufacturer.key}"
crt="${MANUFACTURER_CRT:-${conf_dir}/manufacturer.crt}"

generate_cert "${key}" "${crt}" "${subj}"

"$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/generate-device-ca-certs.sh"
"$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/generate-owner-certs.sh"
