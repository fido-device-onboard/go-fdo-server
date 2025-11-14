#!/bin/bash
# shellcheck disable=SC2034

set -euo pipefail

# These tests are run against the FDO client/servers from the RPM
# packages. Therefore the tests should use the configurations provided
# by the RPMs by default. Override the configuration settings from the
# CI tests by setting them to the RPM default values. The server
# configurations provided by the RPMs can be found in configs
# subdirectory.  In a perfect world this script would extract settings
# from those files, but RHEL lacks a good yaml parsing tool like `yq`
# so for now I risk my immortal soul to an eternity in Programmer's
# Hell and hardcode these values here:

# default key pathnames from configuration files
certs_dir="/etc/pki/go-fdo-server"
device_ca_crt="${certs_dir}/device-ca.crt"
device_ca_key="${certs_dir}/device-ca.key"
device_ca_pub="${certs_dir}/device-ca.pub"

manufacturer_key="${certs_dir}/manufacturer.key"
manufacturer_crt="${manufacturer_key/\.key/.crt}"
manufacturer_pub="${manufacturer_key/\.key/.pub}"
manufacturer_https_key="${certs_dir}/manufacturer-http.key"
manufacturer_https_crt="${certs_dir}/manufacturer-http.crt"

owner_key="${certs_dir}/owner.key"
owner_crt="${owner_key/\.key/.crt}"
owner_pub="${owner_key/\.key/.pub}"
owner_https_key="${certs_dir}/owner-http.key"
owner_https_crt="${certs_dir}/owner-http.crt"

rendezvous_https_key="${certs_dir}/rendezvous-http.key"
rendezvous_https_crt="${certs_dir}/rendezvous-http.crt"

# Default configuration file directory and names from RPM
configs_dir="/etc/go-fdo-server"
manufacturer_config_file="${configs_dir}/manufacturing-server.yaml"
rendezvous_config_file="${configs_dir}/rendezvous-server.yaml"
owner_config_file="${configs_dir}/owner-server.yaml"

# We don't need to generate the certificates as they are generated
# by the systemd services if they don't exist
generate_certs() {
  return
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
  sudo systemctl start go-fdo-server-manufacturer
}

start_service_rendezvous() {
  sudo systemctl start go-fdo-server-rendezvous
}

start_service_owner() {
  sudo systemctl start go-fdo-server-owner
}

stop_service_manufacturer() {
  sudo systemctl stop go-fdo-server-manufacturer
}

stop_service_rendezvous() {
  sudo systemctl stop go-fdo-server-rendezvous
}

stop_service_owner() {
  sudo systemctl stop go-fdo-server-owner
}

# The default server configuration does not enable HTTPS. If the HTTPS
# transport is to be used ensure that a cert/key pair exist and modify
# the systemd configuration to enable HTTPS.
configure_services() {
  for service in "${services[@]}"; do
    local proto_var="${service}_protocol"
    # Safely read protocol with set -u
    local proto_val="${!proto_var-}"
    [[ "${proto_val}" == "https" ]] || continue

    # Check if the cert and key file need to be generated
    local key_var="${service}_https_key"
    local crt_var="${service}_https_crt"
    local key_path="${!key_var-}"
    local crt_path="${!crt_var-}"
    if [[ -z "${key_path}" || -z "${crt_path}" ]]; then
      echo "❌ https requires key and cert filepaths: key=${key_path} crt=${crt_path}"
      return 1
    fi

    if [[ ! -f "${key_path}" || ! -f "${crt_path}" ]]; then
      # Missing cert/keys - generate them using PEM format. Stage them
      # in a temp file since the default cert directory requires root
      # privs
      local subj_var="${service}_https_subj"
      local https_subj="/C=US/O=FDO/CN=${service}"
      if [[ -v ${subj_var} ]]; then
        https_subj="${!subj_var}"
      fi
      local tmp_crt="/tmp/go-fdo-server-${service}.$$.$SRANDOM.crt"
      local tmp_key="/tmp/go-fdo-server-${service}.$$.$SRANDOM.key"
      generate_cert "${tmp_key}" "${tmp_crt}" "" "${https_subj}" pem
      if [[ ! -s "${tmp_crt}" || ! -s "${tmp_key}" ]]; then
        echo "❌ generate_cert failed: key=${tmp_key} crt=${tmp_crt}"
        return 1
      fi

      # Ensure ownership for HTTPS certs/keys (best effort)
      local user="go-fdo-server-${service}"
      local group="go-fdo-server"
      sudo install -m 640 "${tmp_key}" "${key_path}"
      sudo install -m 644 "${tmp_crt}" "${crt_path}"
      sudo chown "${user}:${group}" "${key_path}" "${crt_path}" || true
      rm -f "${tmp_key}" "${tmp_crt}"
    fi

    # Turn on HTTPs using these certs via the command line. The command line
    # in the systemd configuration can be modified using a drop in:
    local service_unit="go-fdo-server-${service}.service"
    local add_opts="--http-cert=${crt_path} --http-key=${key_path}"
    [[ "${service}" =~ "owner" ]] && add_opts+=" --to0-insecure-tls"
    sudo systemctl edit "${service_unit}" --runtime --stdin <<EOF
[Service]
Environment=OPTIONS="${add_opts}"
EOF
  done
}

cleanup_services_configuration() {
  for service in "${services[@]}"; do
    # Undo the drop in to restore the vendor-supplied configuration
    local service_unit="go-fdo-server-${service}.service"
    sudo systemctl revert "${service_unit}"
  done
}
