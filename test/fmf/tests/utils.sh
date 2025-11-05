#!/bin/bash

set -euo pipefail

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

# We generate only the HTTPS transport certs (PEM) for services using HTTPS
configure_services() {
  for service in "${services[@]}"; do
    local proto_var="${service}_protocol"
    # Safely read protocol with set -u
    local proto_val="${!proto_var-}"
    [[ "${proto_val}" == "https" ]] || continue
      # Build var names and safely dereference
    local key_var="${service}_https_key"
    local crt_var="${service}_https_crt"
    local subj_var="${service}_https_subj"
    local key_path="${!key_var-}"
    local crt_path="${!crt_var-}"
    local https_subj="/C=US/O=FDO/CN=${service}"
    if [[ -v ${subj_var} ]]; then
      https_subj="${!subj_var}"
    fi
    generate_cert "${key_path}" "${crt_path}" "" "${https_subj}" pem

    if [[ -n "${key_path}" && -n "${crt_path}" ]]; then
      # Install certs/keys into /etc/go-fdo-server
      local dest_dir="/etc/go-fdo-server"
      local dest_key_path="${dest_dir}/${service}-tls.key"
      local dest_crt_path="${dest_dir}/${service}-tls.crt"
      sudo install -m 640 "${key_path}" "${dest_key_path}"
      sudo install -m 644 "${crt_path}" "${dest_crt_path}"

      # Ensure ownership for HTTPS certs/keys (best effort)
      local user="go-fdo-server-${service}"
      local group="go-fdo-server"
      sudo chown "${user}:${group}" "${dest_key_path}" "${dest_crt_path}" || true

      # Create or update sysconfig with ADDITIONAL_OPTS pointing to /etc/go-fdo-server paths
      local add_opts="--http-cert=${dest_crt_path} --http-key=${dest_key_path}"
      local sysconfig_file="/etc/sysconfig/go-fdo-server-${service}"
      [[ "${service}" =~ "owner" ]] && add_opts+=" --to0-insecure-tls"
      if [[ -f "${sysconfig_file}" ]]; then
        sudo sed -i "s|^ADDITIONAL_OPTS=\".*\"|ADDITIONAL_OPTS=\"${add_opts}\"|" "${sysconfig_file}" || true
      else
        echo "ADDITIONAL_OPTS=\"${add_opts}\"" | sudo tee "${sysconfig_file}" >/dev/null
      fi
    fi
  done
}
