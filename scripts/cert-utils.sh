#! /usr/bin/env bash

generate_cert() {
  local key=$1
  local crt=$2
  local pub=$3
  local subj=$4
  if [[ ! -f "${key}" && ! -f "${crt}" && ! -f "${pub}" ]]; then
    [ -d "$(dirname "${key}")" ] || mkdir -p "$(dirname "${key}")"
    [ -d "$(dirname "${crt}")" ] || mkdir -p "$(dirname "${crt}")"
    [ -d "$(dirname "${pub}")" ] || mkdir -p "$(dirname "${pub}")"
    openssl ecparam -name prime256v1 -genkey -outform der -out "${key}"
    openssl req -x509 -key "${key}" -keyform der -subj "${subj}" -days 365 -out "${crt}"
    openssl x509 -in "${crt}" -pubkey -noout -out "${pub}"
    chmod g+r "${key}" "${crt}" "${pub}"
  fi
}


