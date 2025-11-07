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
  dnf copr remove -y @fedora-iot/fedora-iot
}

install_client() {
  # If PACKIT_COPR_RPMS is not defined it means we are running the test
  # locally so we will install the client from the copr repo
  [ -v "PACKIT_COPR_RPMS" ] || rpm -q go-fdo-client &> /dev/null || install_from_copr go-fdo-client
}

uninstall_client() {
  [ -v "PACKIT_COPR_RPMS" ] || sudo dnf remove -y go-fdo-client
}

install_server() {
  # If PACKIT_COPR_RPMS is not defined it means we are running the test
  # locally so we will build and install the RPMs
  if [ ! -v "PACKIT_COPR_RPMS" ]; then
    commit="$(git rev-parse --short HEAD)"
    rpm -q go-fdo-server | grep -q "go-fdo-server.*git${commit}.*" || { \
      make rpm;
      sudo dnf install -y rpmbuild/rpms/{noarch,"$(uname -m)"}/*git"${commit}"*.rpm;
    }
  else
    echo "  - Expected RPMs:  ${PACKIT_COPR_RPMS}"
  fi
  # Make sure the RPMS are installed
  installed_rpms=$(rpm -q --qf "%{nvr}.%{arch} " go-fdo-server{,-{manufacturer,owner,rendezvous}})
  echo "  - Installed RPMs: ${installed_rpms}"
}

uninstall_server() {
  [ -v "PACKIT_COPR_RPMS" ] || sudo dnf remove -y go-fdo-server{,-manufacturer,-owner,-rendezvous}
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

get_go_fdo_server_logs() {
  local role=$1
  journalctl_args=("--no-pager" "--unit" "go-fdo-server-${role}")
  . /etc/os-release
  [[ "${ID}" = "centos" && "${VERSION_ID}" = "9" ]] || journalctl_args+=("--invocation=0")
  journalctl "${journalctl_args[@]}"
}

get_service_logs_manufacturer() {
  get_go_fdo_server_logs manufacturer | tee "${manufacturer_log}"
}

get_service_logs_rendezvous() {
  get_go_fdo_server_logs rendezvous | tee "${rendezvous_log}"
}

get_service_logs_owner() {
  get_go_fdo_server_logs owner | tee "${owner_log}"
}

get_service_logs() {
  local service=$1
  echo "🛑 ❓ '${service}' logs:"
  local get_service_logs_func="get_service_logs_${service}"
  ! declare -F "${get_service_logs_func}" >/dev/null || ${get_service_logs_func}
}

remove_files() {
  echo "⭐ Removing files from '${base_dir:?}'"
  sudo rm -vrf "${base_dir:?}"/*
  echo "⭐ Removing files from '${rpm_sysconfig_dir}'"
  sudo rm -vf "${rpm_sysconfig_dir:?}/go-fdo-server"*
  echo "⭐ Removing files from '${rpm_config_base_dir}'"
  sudo rm -vf "${rpm_config_base_dir:?}"/*
  echo "⭐ Removing files from '${rpm_manufacturer_database_dir}'"
  sudo rm -vf "${rpm_manufacturer_database_dir:?}/"*
  echo "⭐ Removing files from '${rpm_rendezvous_database_dir}'"
  sudo rm -vf "${rpm_rendezvous_database_dir:?}/"*
  echo "⭐ Removing files from '${rpm_owner_database_dir}'"
  sudo rm -vf "${rpm_owner_database_dir:?}/"*
}

on_failure() {
  trap - ERR
  stop_services
  get_logs
  echo "❌ Test FAILED!"
}

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

cleanup_services_configuration() {
  for service in "${services[@]}"; do
    local sysconfig_file="/etc/sysconfig/go-fdo-server-${service}"
    if [[ -f "${sysconfig_file}" ]]; then
      # Reset ADDITIONAL_OPTS back to empty instead of deleting the file
      if grep -q '^ADDITIONAL_OPTS=' "${sysconfig_file}"; then
        sudo sed -i 's|^ADDITIONAL_OPTS=".*"|ADDITIONAL_OPTS=""|' "${sysconfig_file}" || true
      fi
    fi
  done
}