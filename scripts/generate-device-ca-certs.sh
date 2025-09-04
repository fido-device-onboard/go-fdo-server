#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

ENV_FILE="/etc/sysconfig/go-fdo-server-device-ca"
[ ! -f "${ENV_FILE}" ] || source "${ENV_FILE}"

conf_dir="${DEVICE_CA_CONF_DIR:-/etc/go-fdo-server}"
subj="${DEVICE_CA_SUBJECT:-/C=US/O=FDO/CN=Device CA}"
key="${DEVICE_CA_KEY:-${conf_dir}/device-ca.key}"
crt="${DEVICE_CA_CRT:-${conf_dir}/device-ca.crt}"
pub="${DEVICE_CA_PUB:-${conf_dir}/device-ca.pub}"

generate_cert "${key}" "${crt}" "${pub}" "${subj}"
