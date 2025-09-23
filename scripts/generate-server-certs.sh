#! /bin/bash

source "$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/cert-utils.sh"

ENV_FILE="/etc/sysconfig/go-fdo-server-owner"
[ ! -f "${ENV_FILE}" ] || source "${ENV_FILE}"

cn=$1

server_subj="${SERVER_SUBJECT:-/C=US/O=FDO/CN=${cn} Server}"
server_key="${SERVER_KEY:-${conf_dir}/${cn}.key}"
server_crt="${SERVER_CRT:-${conf_dir}/${cn}.crt}"

generate_cert "${server_key}" "${server_crt}" "${server_subj}"

