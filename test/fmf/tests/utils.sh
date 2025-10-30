#!/bin/bash

set -euo pipefail

# USE_TLS should be set before sourcing this file
: "${USE_TLS:=false}"

# Set up curl insecure flag for TLS mode
if [ "$USE_TLS" = true ]; then
  export CURL_INSECURE_FLAG="--insecure"
else
  export CURL_INSECURE_FLAG=""
fi

# Generate TLS certificates for HTTPS transport (TLS mode only)
# These are separate from the FDO protocol certificates (manufacturer/owner keys)
generate_tls_cert() {
  local service_name=$1
  local conf_dir=$2
  local subj=$3
  local user="go-fdo-server-${service_name}"
  local group="go-fdo-server"
  
  local key="${conf_dir}/${service_name}-tls.key"
  local crt="${conf_dir}/${service_name}-tls.crt"
  
  if [[ ! -f "${key}" && ! -f "${crt}" ]]; then
    [ -d "${conf_dir}" ] || sudo mkdir -p "${conf_dir}"
    
    # Generate private key in PEM format (required by Go's TLS)
    openssl ecparam -name prime256v1 -genkey -out "${key}"
    
    # Generate self-signed certificate
    openssl req -new -x509 -key "${key}" -subj "${subj}" -days 365 -out "${crt}"
    
    # Set ownership to the service user and go-fdo-server group
    sudo chown "${user}:${group}" "${key}" "${crt}"
    sudo chmod 640 "${key}"
    sudo chmod 644 "${crt}"
  fi
}

generate_tls_certs() {
  local conf_dir="/etc/go-fdo-server"
  
  echo "  ⭐ Generating TLS certificates for HTTPS transport"
  generate_tls_cert "manufacturer" "${conf_dir}" "/C=US/O=FDO/CN=manufacturer"
  generate_tls_cert "rendezvous" "${conf_dir}" "/C=US/O=FDO/CN=rendezvous"
  generate_tls_cert "owner" "${conf_dir}" "/C=US/O=FDO/CN=owner"
}

# Override generate_certs to handle TLS cert generation for FMF tests
# FMF tests use systemd services which generate FDO protocol certs automatically,
# but we need to generate TLS transport certs when USE_TLS=true
generate_certs() {
  if [ "$USE_TLS" = true ]; then
    generate_tls_certs
  fi
  # For non-TLS or after TLS cert generation, we don't need to do anything
  # as systemd services will generate FDO protocol certificates if they don't exist
}

# Configure TLS for the services (TLS mode only)
configure_tls() {
  local service=$1
  local conf_dir="/etc/go-fdo-server"
  local sysconfig_file="/etc/sysconfig/go-fdo-server-${service}"
  
  # Modify sysconfig file to set TLS certificate paths
  sudo sed -i "s|^TLS_OPTS=\"\"|TLS_OPTS=\"--server-cert-path=${conf_dir}/${service}-tls.crt --server-key-path=${conf_dir}/${service}-tls.key\"|" "${sysconfig_file}"
}

# Cleanup TLS configuration
cleanup_tls() {
  local service=$1
  local sysconfig_file="/etc/sysconfig/go-fdo-server-${service}"
  
  # Restore sysconfig file to clear TLS_OPTS
  sudo sed -i 's|^TLS_OPTS=".*"|TLS_OPTS=""|' "${sysconfig_file}"
}

install_from_copr() {
  rpm -q --whatprovides 'dnf-command(copr)' &> /dev/null || sudo dnf install -y 'dnf-command(copr)'
  dnf copr list | grep 'fedora-iot/fedora-iot' || sudo dnf copr enable -y @fedora-iot/fedora-iot
  sudo dnf install -y "${@}"
}

install_client() {
  rpm -q go-fdo-client &> /dev/null || install_from_copr go-fdo-client
}

uninstall_client() {
  sudo dnf remove -y go-fdo-client
}

install_server() {
  rpm -q go-fdo-server-{manufacturer,owner,rendezvous} || install_from_copr go-fdo-server{,-manufacturer,-owner,-rendezvous}
}

uninstall_server() {
  sudo dnf remove -y go-fdo-server{,-manufacturer,-owner,-rendezvous}
}

start_service_manufacturer() {
  if [ "$USE_TLS" = true ]; then
    configure_tls manufacturer
  fi
  sudo systemctl start go-fdo-server-manufacturer
}

start_service_rendezvous() {
  if [ "$USE_TLS" = true ]; then
    configure_tls rendezvous
  fi
  sudo systemctl start go-fdo-server-rendezvous
}

start_service_owner() {
  if [ "$USE_TLS" = true ]; then
    configure_tls owner
  fi
  sudo systemctl start go-fdo-server-owner
}

stop_service_manufacturer() {
  sudo systemctl stop go-fdo-server-manufacturer
  if [ "$USE_TLS" = true ]; then
    cleanup_tls manufacturer
  fi
}

stop_service_rendezvous() {
  sudo systemctl stop go-fdo-server-rendezvous
  if [ "$USE_TLS" = true ]; then
    cleanup_tls rendezvous
  fi
}

stop_service_owner() {
  sudo systemctl stop go-fdo-server-owner
  if [ "$USE_TLS" = true ]; then
    cleanup_tls owner
  fi
}

# Override URLs for TLS mode
if [ "$USE_TLS" = true ]; then
  manufacturer_url="https://${manufacturer_service}"
  manufacturer_health_url="${manufacturer_url}/health"
  
  rendezvous_url="https://${rendezvous_service}"
  rendezvous_health_url="${rendezvous_url}/health"
  
  owner_url="https://${owner_service}"
  owner_health_url="${owner_url}/health"
  
fi
