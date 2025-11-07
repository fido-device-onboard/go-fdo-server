#! /usr/bin/env bash

generate_cert() {
  local key=$1
  local crt=$2
  local subj=$3
  local form=${4:-der}
  if [[ ! -f "${key}" && ! -f "${crt}" ]]; then
    [ -d "$(dirname "${key}")" ] || mkdir -p "$(dirname "${key}")"
    [ -d "$(dirname "${crt}")" ] || mkdir -p "$(dirname "${crt}")"
    openssl ecparam -name prime256v1 -genkey -outform "${form}" -out "${key}"
    openssl req -x509 -key "${key}" -keyform "${form}" -subj "${subj}" -days 365 -out "${crt}"
  fi
}

extract_pubkey_from_cert() {
  local crt=$1
  local pub=$2
  if [[ ! -f "${pub}" ]]; then
    [ -d "$(dirname "${pub}")" ] || mkdir -p "$(dirname "${pub}")"
    openssl x509 -in "${crt}" -pubkey -noout -out "${pub}"
  fi
}
