#! /usr/bin/env bash

generate_cert() {
  local key=$1
  local crt=$2
  local pub=$3
  local subj=$4
  local format=${5:-der} # der (default for FDO keys) or pem (for HTTPS keys)
  # Determine whether to generate a pub file (skip if pub is empty)
  local gen_pub=1
  if [[ -z "${pub}" ]]; then
    gen_pub=0
  fi
  # Check existing files conditionally
  if [[ ! -f "${key}" && ! -f "${crt}" && ( ${gen_pub} -eq 0 || ! -f "${pub}" ) ]]; then
    [ -d "$(dirname "${key}")" ] || mkdir -p "$(dirname "${key}")"
    [ -d "$(dirname "${crt}")" ] || mkdir -p "$(dirname "${crt}")"
    if [[ ${gen_pub} -eq 1 ]]; then
      [ -d "$(dirname "${pub}")" ] || mkdir -p "$(dirname "${pub}")"
    fi
    if [[ "${format}" = "pem" ]]; then
      # Generate PEM EC private key and self-signed certificate (suitable for HTTPS)
      openssl ecparam -name prime256v1 -genkey -out "${key}"
      openssl req -x509 -key "${key}" -subj "${subj}" -days 365 -out "${crt}"
    else
      # Generate DER EC private key and self-signed certificate (used by FDO keys)
      openssl ecparam -name prime256v1 -genkey -outform der -out "${key}"
      openssl req -x509 -key "${key}" -keyform der -subj "${subj}" -days 365 -out "${crt}"
    fi
    if [[ ${gen_pub} -eq 1 ]]; then
      openssl x509 -in "${crt}" -pubkey -noout -out "${pub}"
      chmod g+r "${pub}"
    fi
    chmod g+r "${key}" "${crt}"
  fi
}


