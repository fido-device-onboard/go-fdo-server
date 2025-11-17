#!/bin/bash
# shellcheck disable=SC2034

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../ci/utils.sh"

rpm_config_base_dir="/etc/go-fdo-server"
rpm_certs_base_dir="/etc/pki/go-fdo-server"
rpm_systemd_drop_in_dir="/run/systemd/system"
rpm_group="go-fdo-server"

rpm_device_ca_user="go-fdo-server-manufacturer"
rpm_device_ca_crt="${rpm_certs_base_dir}/device-ca-example.crt"
rpm_device_ca_key="${rpm_certs_base_dir}/device-ca-example.key"

rpm_manufacturer_user="go-fdo-server-manufacturer"
rpm_manufacturer_db_type="sqlite"
rpm_manufacturer_database_dir="/var/lib/go-fdo-server-manufacturer"
rpm_manufacturer_db_dsn="file:${rpm_manufacturer_database_dir}/db.sqlite"
rpm_manufacturer_key="${rpm_certs_base_dir}/manufacturer-example.key"
rpm_manufacturer_crt="${rpm_certs_base_dir}/manufacturer-example.crt"
rpm_manufacturer_https_key="${rpm_certs_base_dir}/manufacturer-https-example.key"
rpm_manufacturer_https_crt="${rpm_certs_base_dir}/manufacturer-https-example.crt"

rpm_rendezvous_user="go-fdo-server-rendezvous"
rpm_rendezvous_db_type="sqlite"
rpm_rendezvous_database_dir="/var/lib/go-fdo-server-rendezvous"
rpm_rendezvous_db_dsn="file:${rpm_rendezvous_database_dir}/db.sqlite"
rpm_rendezvous_https_key="${rpm_certs_base_dir}/rendezvous-https-example.key"
rpm_rendezvous_https_crt="${rpm_certs_base_dir}/rendezvous-https-example.crt"

rpm_owner_user="go-fdo-server-owner"
rpm_owner_db_type="sqlite"
rpm_owner_database_dir="/var/lib/go-fdo-server-owner"
rpm_owner_db_dsn="file:${rpm_owner_database_dir}/db.sqlite"
rpm_owner_key="${rpm_certs_base_dir}/owner-example.key"
rpm_owner_crt="${rpm_certs_base_dir}/owner-example.crt"
rpm_owner_https_key="${rpm_certs_base_dir}/owner-https-example.key"
rpm_owner_https_crt="${rpm_certs_base_dir}/owner-https-example.crt"

configure_service_manufacturer() {
  sudo bash -c "
  mkdir -p ${rpm_certs_base_dir}
  cp ${manufacturer_key} ${rpm_manufacturer_key}
  cp ${manufacturer_crt} ${rpm_manufacturer_crt}
  chown ${rpm_manufacturer_user}:${rpm_group} ${rpm_manufacturer_key} ${rpm_manufacturer_crt}
  chmod g+r ${rpm_manufacturer_crt}

  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  # Add additional options to manufacturer if https is used
  if [ '${manufacturer_protocol}' = 'https' ]; then
    cp ${manufacturer_https_key} ${rpm_manufacturer_https_key}
    cp ${manufacturer_https_crt} ${rpm_manufacturer_https_crt}
    chown ${rpm_manufacturer_user}:${rpm_group} ${rpm_manufacturer_https_key} ${rpm_manufacturer_https_crt}
    mkdir -p ${rpm_systemd_drop_in_dir}/go-fdo-server-manufacturer.service.d
    cat <<EOF | tee ${rpm_systemd_drop_in_dir}/go-fdo-server-manufacturer.service.d/10-https.conf
[Service]
ExecStart=
ExecStart=/usr/bin/go-fdo-server manufacturing --http-cert=${rpm_manufacturer_https_crt} --http-key=${rpm_manufacturer_https_key}
EOF
    systemctl daemon-reload
  fi
  "
}

configure_service_rendezvous() {
  sudo bash -c "
  mkdir -p ${rpm_certs_base_dir}
  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  # Add additional options to rendezvous if https is used
  if [ '${rendezvous_protocol}' = 'https' ]; then
    cp ${rendezvous_https_key} ${rpm_rendezvous_https_key}
    cp ${rendezvous_https_crt} ${rpm_rendezvous_https_crt}
    chown ${rpm_rendezvous_user}:${rpm_group} ${rpm_rendezvous_https_key} ${rpm_rendezvous_https_crt}
    mkdir -p ${rpm_systemd_drop_in_dir}/go-fdo-server-rendezvous.service.d
    cat <<EOF | tee ${rpm_systemd_drop_in_dir}/go-fdo-server-rendezvous.service.d/10-https.conf
[Service]
ExecStart=
ExecStart=/usr/bin/go-fdo-server rendezvous --http-cert=${rpm_rendezvous_https_crt} --http-key=${rpm_rendezvous_https_key}
EOF
    systemctl daemon-reload
  fi
  "
}

configure_service_owner() {
  sudo bash -c "
  mkdir -p ${rpm_certs_base_dir}
  cp ${owner_key} ${rpm_owner_key}
  cp ${owner_crt} ${rpm_owner_crt}
  chown ${rpm_owner_user}:${rpm_group} ${rpm_owner_key} ${rpm_owner_crt}
  chmod g+r ${rpm_owner_crt}

  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  # Add additional options to owner if https is used
  if [ '${owner_protocol}' = 'https' ]; then
    cp ${owner_https_key} ${rpm_owner_https_key}
    cp ${owner_https_crt} ${rpm_owner_https_crt}
    chown ${rpm_owner_user}:${rpm_group} ${rpm_owner_https_key} ${rpm_owner_https_crt}
    mkdir -p ${rpm_systemd_drop_in_dir}/go-fdo-server-owner.service.d
    cat <<EOF | tee ${rpm_systemd_drop_in_dir}/go-fdo-server-owner.service.d/10-https.conf
[Service]
ExecStart=
ExecStart=/usr/bin/go-fdo-server owner --http-cert=${rpm_owner_https_crt} --http-key=${rpm_owner_https_key} --to0-insecure-tls
EOF
    systemctl daemon-reload
  fi
  "
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

# We do not use pid files but functions to stop the services via systemctl
stop_service() {
  local service=$1
  echo -n "  ⚙ Stopping service ${service} "
  local stop_service="stop_service_${service}"
  ! declare -F "${stop_service}" >/dev/null || ${stop_service}
  echo " ✔"
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

cleanup_drop_ins() {
  local reload_systemd=0
  for service in rendezvous manufacturer owner; do
    local drop_in_dir="${rpm_systemd_drop_in_dir:?}/go-fdo-server-${service}.service.d"
    if [[ -d "${drop_in_dir}" ]]; then
      reload_systemd=1
      sudo rm -vrf "${drop_in_dir}"
    fi
  done
  if [[ ${reload_systemd} -eq 1 ]]; then
    sudo systemctl daemon-reload
  fi
}

remove_files() {
  echo "⭐ Removing files from '${base_dir:?}'"
  sudo rm -vrf "${base_dir:?}"/*
  echo "⭐ Removing files from '${rpm_config_base_dir}'"
  sudo rm -vf "${rpm_config_base_dir:?}"/*
  echo "⭐ Removing files from '${rpm_certs_base_dir}'"
  sudo rm -vf "${rpm_certs_base_dir:?}/"*
  echo "⭐ Removing files from '${rpm_manufacturer_database_dir}'"
  sudo rm -vf "${rpm_manufacturer_database_dir:?}/"*
  echo "⭐ Removing files from '${rpm_rendezvous_database_dir}'"
  sudo rm -vf "${rpm_rendezvous_database_dir:?}/"*
  echo "⭐ Removing files from '${rpm_owner_database_dir}'"
  sudo rm -vf "${rpm_owner_database_dir:?}/"*
  echo "⭐ Removing systemd drop-in files"
  cleanup_drop_ins
}

on_failure() {
  trap - ERR
  stop_services
  get_logs
  echo "❌ Test FAILED!"
}
