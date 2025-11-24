#!/bin/bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../ci/utils.sh"

rpm_config_base_dir="/etc/go-fdo-server"
rpm_sysconfig_dir="/etc/sysconfig"
rpm_group="go-fdo-server"

rpm_device_ca_user="go-fdo-server-manufacturer"
rpm_device_ca_crt="${rpm_config_base_dir}/device-ca.crt"
rpm_device_ca_key="${rpm_config_base_dir}/device-ca.key"

rpm_manufacturer_user="go-fdo-server-manufacturer"
rpm_manufacturer_db_type="sqlite"
rpm_manufacturer_database_dir="/var/lib/go-fdo-server-manufacturer"
rpm_manufacturer_db_dsn="file:${rpm_manufacturer_database_dir}/db.sqlite"
rpm_manufacturer_sysconfig_file="${rpm_sysconfig_dir}/go-fdo-server-manufacturer"
rpm_manufacturer_key="${rpm_config_base_dir}/manufacturer.key"
rpm_manufacturer_crt="${rpm_config_base_dir}/manufacturer.crt"
rpm_manufacturer_https_key="${rpm_config_base_dir}/manufacturer-https.key"
rpm_manufacturer_https_crt="${rpm_config_base_dir}/manufacturer-https.crt"

rpm_rendezvous_user="go-fdo-server-rendezvous"
rpm_rendezvous_db_type="sqlite"
rpm_rendezvous_database_dir="/var/lib/go-fdo-server-rendezvous"
rpm_rendezvous_db_dsn="file:${rpm_rendezvous_database_dir}/db.sqlite"
rpm_rendezvous_sysconfig_file="${rpm_sysconfig_dir}/go-fdo-server-rendezvous"
rpm_rendezvous_https_key="${rpm_config_base_dir}/rendezvous-https.key"
rpm_rendezvous_https_crt="${rpm_config_base_dir}/rendezvous-https.crt"

rpm_owner_user="go-fdo-server-owner"
rpm_owner_db_type="sqlite"
rpm_owner_database_dir="/var/lib/go-fdo-server-owner"
rpm_owner_db_dsn="file:${rpm_owner_database_dir}/db.sqlite"
rpm_owner_sysconfig_file="${rpm_sysconfig_dir}/go-fdo-server-owner"
rpm_owner_key="${rpm_config_base_dir}/owner.key"
rpm_owner_crt="${rpm_config_base_dir}/owner.crt"
rpm_owner_https_key="${rpm_config_base_dir}/owner-https.key"
rpm_owner_https_crt="${rpm_config_base_dir}/owner-https.crt"

configure_service_manufacturer() {
  sudo bash -c "
  cp ${manufacturer_key} ${rpm_manufacturer_key}
  cp ${manufacturer_crt} ${rpm_manufacturer_crt}
  chown ${rpm_manufacturer_user}:${rpm_group} ${rpm_manufacturer_key} ${rpm_manufacturer_crt}
  chmod g+r ${rpm_manufacturer_crt}

  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  > ${rpm_manufacturer_sysconfig_file}
  echo 'LISTEN_IP=0.0.0.0'                         >> ${rpm_manufacturer_sysconfig_file}
  echo 'LISTEN_PORT=${manufacturer_port}'          >> ${rpm_manufacturer_sysconfig_file}
  echo 'DATABASE_TYPE=${rpm_manufacturer_db_type}' >> ${rpm_manufacturer_sysconfig_file}
  echo 'DATABASE_DSN=${rpm_manufacturer_db_dsn}'   >> ${rpm_manufacturer_sysconfig_file}
  echo 'MANUFACTURER_KEY=${rpm_manufacturer_key}'  >> ${rpm_manufacturer_sysconfig_file}
  echo 'OWNER_CRT=${rpm_owner_crt}'                >> ${rpm_manufacturer_sysconfig_file}
  echo 'DEVICE_CA_CRT=${rpm_device_ca_crt}'        >> ${rpm_manufacturer_sysconfig_file}
  echo 'DEVICE_CA_KEY=${rpm_device_ca_key}'        >> ${rpm_manufacturer_sysconfig_file}

  additional_opts='--log-level=debug'
  # Add additional options to manufacturer if https is used
  if [ '${manufacturer_protocol}' = 'https' ]; then
    cp ${manufacturer_https_key} ${rpm_manufacturer_https_key}
    cp ${manufacturer_https_crt} ${rpm_manufacturer_https_crt}
    chown ${rpm_manufacturer_user}:${rpm_group} ${rpm_manufacturer_https_key} ${rpm_manufacturer_https_crt}
    additional_opts=\"\${additional_opts} --http-cert=${rpm_manufacturer_https_crt} --http-key=${rpm_manufacturer_https_key}\"
  fi
  echo ADDITIONAL_OPTS=\\\"\${additional_opts}\\\" >> ${rpm_manufacturer_sysconfig_file}
  "
}

configure_service_rendezvous() {
  sudo bash -c "
  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  > ${rpm_rendezvous_sysconfig_file}
  echo 'LISTEN_IP=0.0.0.0'                       >> ${rpm_rendezvous_sysconfig_file}
  echo 'LISTEN_PORT=${rendezvous_port}'          >> ${rpm_rendezvous_sysconfig_file}
  echo 'DATABASE_TYPE=${rpm_rendezvous_db_type}' >> ${rpm_rendezvous_sysconfig_file}
  echo 'DATABASE_DSN=${rpm_rendezvous_db_dsn}'   >> ${rpm_rendezvous_sysconfig_file}

  additional_opts='--log-level=debug'
  # Add additional options to rendezvous if https is used
  if [ '${rendezvous_protocol}' = 'https' ]; then
    cp ${rendezvous_https_key} ${rpm_rendezvous_https_key}
    cp ${rendezvous_https_crt} ${rpm_rendezvous_https_crt}
    chown ${rpm_rendezvous_user}:${rpm_group} ${rpm_rendezvous_https_key} ${rpm_rendezvous_https_crt}
    additional_opts=\"\${additional_opts} --http-cert=${rpm_rendezvous_https_crt} --http-key=${rpm_rendezvous_https_key}\"
  fi
  echo ADDITIONAL_OPTS=\\\"\${additional_opts}\\\" >> ${rpm_rendezvous_sysconfig_file}
  "
}


configure_service_owner() {
  sudo bash -c "
  cp ${owner_key} ${rpm_owner_key}
  cp ${owner_crt} ${rpm_owner_crt}
  chown ${rpm_owner_user}:${rpm_group} ${rpm_owner_key} ${rpm_owner_crt}
  chmod g+r ${rpm_owner_crt}

  cp ${device_ca_crt} ${rpm_device_ca_crt}
  cp ${device_ca_key} ${rpm_device_ca_key}

  chown ${rpm_device_ca_user}:${rpm_group} ${rpm_device_ca_key} ${rpm_device_ca_crt}
  chmod g+r ${rpm_device_ca_crt}

  > ${rpm_owner_sysconfig_file}
  echo 'LISTEN_IP=0.0.0.0'                  >> ${rpm_owner_sysconfig_file}
  echo 'LISTEN_PORT=${owner_port}'          >> ${rpm_owner_sysconfig_file}
  echo 'DATABASE_TYPE=${rpm_owner_db_type}' >> ${rpm_owner_sysconfig_file}
  echo 'DATABASE_DSN=${rpm_owner_db_dsn}'   >> ${rpm_owner_sysconfig_file}
  echo 'OWNER_KEY=${rpm_owner_key}'         >> ${rpm_owner_sysconfig_file}
  echo 'OWNER_CRT=${rpm_owner_crt}'         >> ${rpm_owner_sysconfig_file}
  echo 'DEVICE_CA_CRT=${rpm_device_ca_crt}' >> ${rpm_owner_sysconfig_file}

  additional_opts='--log-level=debug'
  # Add additional options to owner if https is used
  if [ '${owner_protocol}' = 'https' ]; then
    cp ${owner_https_key} ${rpm_owner_https_key}
    cp ${owner_https_crt} ${rpm_owner_https_crt}
    chown ${rpm_owner_user}:${rpm_group} ${rpm_owner_https_key} ${rpm_owner_https_crt}
    additional_opts=\"\${additional_opts} --http-cert=${rpm_owner_https_crt} --http-key=${rpm_owner_https_key} --to0-insecure-tls\"
  fi
    echo ADDITIONAL_OPTS=\\\"\${additional_opts}\\\" >> ${rpm_owner_sysconfig_file}
  "
}

install_from_copr() {
  rpm -q --whatprovides 'dnf-command(copr)' &> /dev/null || sudo dnf install -y 'dnf-command(copr)'
  dnf copr list | grep 'fedora-iot/fedora-iot' || sudo dnf copr enable -y @fedora-iot/fedora-iot
  # testing-farm-tag-repository is causing problems with builds see:
  # https://docs.testing-farm.io/Testing%20Farm/0.1/test-environment.html#disabling-tag-repository
  sudo dnf install -y "$@"
  sudo dnf copr disable -y @fedora-iot/fedora-iot
}

install_client() {
  # If PACKIT_COPR_RPMS is not defined it means we are running the test
  # locally so we will install the client from the copr repo
  [ -v "PACKIT_COPR_RPMS" ] || rpm -q go-fdo-client &> /dev/null || install_from_copr go-fdo-client
}

uninstall_client() {
  # When running a test locally we remove the client package
  # after a successful execution.
  [ -v "PACKIT_COPR_RPMS" ] || {
    sudo dnf remove -y go-fdo-client;
    sudo dnf copr remove -y @fedora-iot/fedora-iot;
  }
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
  local stop_service="stop_service_${service}"
  ! declare -F "${stop_service}" >/dev/null || ${stop_service}
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
  journalctl_args=("--no-pager" "--output=cat" "--unit" "go-fdo-server-${role}")
  . /etc/os-release
  [[ "${ID}" = "centos" && "${VERSION_ID}" = "9" ]] || journalctl_args+=("--invocation=0")
  systemctl status "go-fdo-server-${role}.service" || true
  journalctl "${journalctl_args[@]}"
}

get_service_logs_manufacturer() {
  get_go_fdo_server_logs manufacturer
}

get_service_logs_rendezvous() {
  get_go_fdo_server_logs rendezvous
}

get_service_logs_owner() {
  get_go_fdo_server_logs owner
}

get_service_logs() {
  local service=$1
  log "🛑 '${service}' logs:\n"
  local get_service_logs_func="get_service_logs_${service}"
  ! declare -F "${get_service_logs_func}" >/dev/null || ${get_service_logs_func}
}

save_go_fdo_server_logs() {
  local role=$1
  local log_file=$2
  get_go_fdo_server_logs "${role}" > "${log_file}"
}

save_service_logs_manufacturer() {
  save_go_fdo_server_logs manufacturer "${manufacturer_log}"
}

save_service_logs_rendezvous() {
  save_go_fdo_server_logs rendezvous "${rendezvous_log}"
}

save_service_logs_owner() {
  save_go_fdo_server_logs owner "${owner_log}"
}

save_service_logs() {
  local service=$1
  log "\t⚙ Saving '${service}' logs "
  local save_service_logs_func="save_service_logs_${service}"
  ! declare -F "${save_service_logs_func}" >/dev/null || ${save_service_logs_func}
  log_success
}

save_logs() {
  log_info "Saving logs"
  for service in "${services[@]}"; do
    save_service_logs ${service}
  done
  if [ -v "PACKIT_COPR_RPMS" ]; then
    log_info "Submitting files to TMT '${base_dir:?}'"
    find "${base_dir:?}" -type f -exec tmt-file-submit -l {} \;
  fi
}

remove_files() {
  log_info "Removing files from '${base_dir:?}'"
  sudo rm -vrf "${base_dir:?}"/*
  log_info "Removing files from '${rpm_sysconfig_dir}'"
  sudo rm -vf "${rpm_sysconfig_dir:?}/go-fdo-server"/*
  log_info "Removing files from '${rpm_config_base_dir}'"
  sudo rm -vf "${rpm_config_base_dir:?}"/*
  log_info "Removing files from '${rpm_manufacturer_database_dir}'"
  sudo rm -vf "${rpm_manufacturer_database_dir:?}"/*
  log_info "Removing files from '${rpm_rendezvous_database_dir}'"
  sudo rm -vf "${rpm_rendezvous_database_dir:?}"/*
  log_info "Removing files from '${rpm_owner_database_dir}'"
  sudo rm -vf "${rpm_owner_database_dir:?}"/*
}

on_failure() {
  trap - ERR
  save_logs
  stop_services
  test_fail
}

cleanup() {
  [ ! -v "PACKIT_COPR_RPMS" ] || save_logs
  stop_services
  unset_hostnames
  uninstall_server
  uninstall_client
  remove_files
}
