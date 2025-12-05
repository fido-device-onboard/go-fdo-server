#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

ENV_FILE="/etc/sysconfig/go-fdo-server-owner"
[ ! -f "${ENV_FILE}" ] || source "${ENV_FILE}"

conf_dir="${OWNER_CONF_DIR:-/etc/go-fdo-server}"
subj="${OWNER_SUBJECT:-/C=US/O=FDO/CN=Owner}"
key="${OWNER_KEY:-${conf_dir}/owner.key}"
crt="${OWNER_CRT:-${conf_dir}/owner.crt}"

generate_cert "${key}" "${crt}" "${subj}"

"$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/generate-device-ca-certs.sh"
