#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

set -x
echo `whoami`
ls -ld /etc || echo "no /etc"
ls -ld /etc/go-fdo-server || echo "no /etc/go-fdo-server"
ls -l /etc/go-fdo-server || echo "can not ls"

ENV_FILE="/etc/sysconfig/go-fdo-server-manufacturer"
[ ! -f "${ENV_FILE}" ] || source "${ENV_FILE}"

conf_dir="${MANUFACTURER_CONF_DIR:-/etc/go-fdo-server}"
subj="${MANUFACTURER_SUBJECT:-/C=US/O=FDO/CN=Manufacturer}"
key="${MANUFACTURER_KEY:-${conf_dir}/manufacturer.key}"
crt="${MANUFACTURER_CRT:-${conf_dir}/manufacturer.crt}"
pub="${MANUFACTURER_PUB:-${conf_dir}/manufacturer.pub}"

generate_cert "${key}" "${crt}" "${pub}" "${subj}"

"$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/generate-device-ca-certs.sh"
"$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/generate-owner-certs.sh"
