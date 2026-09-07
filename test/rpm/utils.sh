#!/bin/bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../ci/utils.sh"

# PLEASE READ:
#
# The FMF tests deploy the FDO servers via RPM packages. These
# packages are either provided by the Packit service or pulled from
# the fedora-iot COPR repository.
#
# All test-related configuration resides under the ${base_dir} working
# directory, which is made available as a testing artifact for
# debugging purposes. The FDO server configuration files also reside
# in ${base_dir} and are copied into the server configuration search
# path at the start of the test.
#
# If a test requires a custom configuration consider the following
# recommendations:
#
# o) Keep all test configuration in ${base_dir} in order to get it
# included in the testing artifacts.
#
# o) To create a custom configuration file copy the necessary
# `generate_${service}_config()` function(s) to your test and modify
# them to produce the desired test configuration. Your generated
# configuration file will automatically be saved in a directory that
# takes precedence over the default in the server's configuration
# search path. A copy of configuration file will be available
# as a testing artifact end of the test.
#
# o) To pass command line arguments to the server create a systemd
# "drop-in" file that overrides the ExecStart= setting in the service
# file. This drop-in file should be placed in the directory defined by
# the variable `systemd_${service}_drop_in_dir`.  Remember to run
# `sudo systemctl daemon-reload` after writing the drop-in file.

installation_source="${INSTALLATION_SOURCE:-}"
client_installation_source="${CLIENT_INSTALLATION_SOURCE:-${installation_source}}"
server_installation_source="${SERVER_INSTALLATION_SOURCE:-${installation_source}}"

rpm_certs_dir="/etc/pki/go-fdo-server" # RPMs generate the default certs/keys
rpm_server_group="go-fdo-server"       # server Group ID created by RPM install

rpm_manufacturer_user="go-fdo-server-manufacturer"
rpm_manufacturer_home_dir="/run/go-fdo-server-manufacturer"
rpm_manufacturer_config_dir="${rpm_manufacturer_home_dir}/.config/go-fdo-server"
rpm_manufacturer_config_file="${rpm_manufacturer_config_dir}/manufacturing.yaml"
rpm_manufacturer_db_type="sqlite"
rpm_manufacturer_database_dir="/var/lib/go-fdo-server-manufacturer"
rpm_manufacturer_database_file="${rpm_manufacturer_database_dir}/db.sqlite3"
rpm_manufacturer_db_dsn="file:${rpm_manufacturer_database_file}"
manufacturer_config_file="${configs_dir}/manufacturing.yaml"

rpm_rendezvous_user="go-fdo-server-rendezvous"
rpm_rendezvous_home_dir="/run/go-fdo-server-rendezvous"
rpm_rendezvous_config_dir="${rpm_rendezvous_home_dir}/.config/go-fdo-server"
rpm_rendezvous_config_file="${rpm_rendezvous_config_dir}/rendezvous.yaml"
rpm_rendezvous_db_type="sqlite"
rpm_rendezvous_database_dir="/var/lib/go-fdo-server-rendezvous"
rpm_rendezvous_database_file="${rpm_rendezvous_database_dir}/db.sqlite3"
rpm_rendezvous_db_dsn="file:${rpm_rendezvous_database_file}"
rendezvous_config_file="${configs_dir}/rendezvous.yaml"

rpm_owner_user="go-fdo-server-owner"
rpm_owner_home_dir="/run/go-fdo-server-owner"
rpm_owner_config_dir="${rpm_owner_home_dir}/.config/go-fdo-server"
rpm_owner_config_file="${rpm_owner_config_dir}/owner.yaml"
rpm_owner_db_type="sqlite"
rpm_owner_database_dir="/var/lib/go-fdo-server-owner"
rpm_owner_database_file="${rpm_owner_database_dir}/db.sqlite3"
rpm_owner_db_dsn="file:${rpm_owner_database_file}"
owner_config_file="${configs_dir}/owner.yaml"
owner_reuse_creds="false"
owner_to0_insecure_tls="false"

go_fdo_server_rpms="go-fdo-server go-fdo-server-manufacturer go-fdo-server-owner go-fdo-server-rendezvous"
go_fdo_client_rpms="go-fdo-client"

# systemd drop-in file configuration
#
systemd_drop_in_base_dir="/run/systemd/system"
#shellcheck disable=SC2034
systemd_manufacturer_drop_in_dir="${systemd_drop_in_base_dir}/go-fdo-server-manufacturer.service.d"
#shellcheck disable=SC2034
systemd_rendezvous_drop_in_dir="${systemd_drop_in_base_dir}/go-fdo-server-rendezvous.service.d"
#shellcheck disable=SC2034
systemd_owner_drop_in_dir="${systemd_drop_in_base_dir}/go-fdo-server-owner.service.d"

# Generate a default configuration file for the manufacturing server
# that references resources from the test base working directory
generate_manufacturer_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "${rpm_manufacturer_db_type}"
  dsn: "${rpm_manufacturer_db_dsn}"
manufacturing:
  key: "${rpm_manufacturer_home_dir}/manufacturer.key"
device_ca:
  cert: "${rpm_manufacturer_home_dir}/device_ca.crt"
  key: "${rpm_manufacturer_home_dir}/device_ca.key"
owner:
  cert: "${rpm_manufacturer_home_dir}/owner.crt"
http:
  ip: "${manufacturer_dns}"
  port: "${manufacturer_port}"
EOF
  # Enable HTTP if https protocol is used
  if [ "${manufacturer_protocol}" = "https" ]; then
    cat <<EOF
  cert: "${rpm_manufacturer_home_dir}/manufacturer-http.crt"
  key: "${rpm_manufacturer_home_dir}/manufacturer-http.key"
EOF
  fi
}

# Setup manufacturer home directory, create the configuration and copy
# all necessary certs/keys
configure_service_manufacturer() {
  sudo rm -rf "${rpm_manufacturer_home_dir:?}"
  sudo mkdir -p "${rpm_manufacturer_config_dir}" # creates home dir
  generate_manufacturer_config | tee "${manufacturer_config_file}"
  sudo cp "${manufacturer_config_file}" "${rpm_manufacturer_config_file}"
  sudo cp "${manufacturer_key}" "${rpm_manufacturer_home_dir}"
  sudo cp "${owner_crt}" "${rpm_manufacturer_home_dir}"
  sudo cp "${device_ca_key}" "${rpm_manufacturer_home_dir}"
  sudo cp "${device_ca_crt}" "${rpm_manufacturer_home_dir}"
  if [ "${manufacturer_protocol}" = "https" ]; then
    sudo cp "${manufacturer_https_key}" "${manufacturer_https_crt}" "${rpm_manufacturer_home_dir}"
  fi
  sudo chown -R ${rpm_manufacturer_user}:${rpm_server_group} ${rpm_manufacturer_home_dir}
}

generate_rendezvous_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "${rpm_rendezvous_db_type}"
  dsn: "${rpm_rendezvous_db_dsn}"
http:
  ip: "${rendezvous_dns}"
  port: "${rendezvous_port}"
EOF
  # Enable HTTP if https protocol is used
  if [ "${rendezvous_protocol}" = "https" ]; then
    cat <<EOF
  cert: "${rpm_rendezvous_home_dir}/rendezvous-http.crt"
  key: "${rpm_rendezvous_home_dir}/rendezvous-http.key"
EOF
  fi
}

# Setup rendezvous home directory, create the configuration and copy
# all necessary certs/keys
configure_service_rendezvous() {
  sudo rm -rf "${rpm_rendezvous_home_dir:?}"
  sudo mkdir -p "${rpm_rendezvous_config_dir}" # creates home dir
  generate_rendezvous_config | tee "${rendezvous_config_file}"
  sudo cp "${rendezvous_config_file}" "${rpm_rendezvous_config_file}"
  if [ "${rendezvous_protocol}" = "https" ]; then
    sudo cp "${rendezvous_https_key}" "${rendezvous_https_crt}" "${rpm_rendezvous_home_dir}"
  fi
  sudo chown -R ${rpm_rendezvous_user}:${rpm_server_group} ${rpm_rendezvous_home_dir}
}

generate_owner_config() {
  cat <<EOF
log:
  level: "debug"
db:
  type: "${rpm_owner_db_type}"
  dsn: "${rpm_owner_db_dsn}"
device_ca:
  cert: "${rpm_owner_home_dir}/device_ca.crt"
owner:
  cert: "${rpm_owner_home_dir}/owner.crt"
  key: "${rpm_owner_home_dir}/owner.key"
  reuse_credentials: "${owner_reuse_creds}"
  to0_insecure_tls: "${owner_to0_insecure_tls}"
http:
  ip: "${owner_dns}"
  port: "${owner_port}"
EOF
  # Enable HTTP if https protocol is used
  if [ "${owner_protocol}" = "https" ]; then
    cat <<EOF
  cert: "${rpm_owner_home_dir}/owner-http.crt"
  key: "${rpm_owner_home_dir}/owner-http.key"
EOF
  fi
}

# Setup owner home directory, create the configuration and copy
# all necessary certs/keys
configure_service_owner() {
  sudo rm -rf "${rpm_owner_home_dir:?}"
  sudo mkdir -p "${rpm_owner_config_dir}" # creates home dir
  generate_owner_config | tee "${owner_config_file}"
  sudo cp "${owner_config_file}" "${rpm_owner_config_file}"
  sudo cp "${device_ca_crt}" "${rpm_owner_home_dir}"
  sudo cp "${owner_crt}" "${owner_key}" "${rpm_owner_home_dir}"
  if [ "${owner_protocol}" = "https" ]; then
    sudo cp "${owner_https_key}" "${owner_https_crt}" "${rpm_owner_home_dir}"
  fi
  sudo chown -R ${rpm_owner_user}:${rpm_server_group} ${rpm_owner_home_dir}
}

install_from_copr() {
  rpm -q --whatprovides 'dnf-command(copr)' &>/dev/null || sudo dnf install -y 'dnf-command(copr)'
  dnf copr list | grep 'fedora-iot/fedora-iot' || sudo dnf copr enable -y @fedora-iot/fedora-iot
  # testing-farm-tag-repository is causing problems with builds see:
  # https://docs.testing-farm.io/Testing%20Farm/0.1/test-environment.html#disabling-tag-repository
  sudo dnf install --disablerepo=* --enablerepo=copr:copr.fedorainfracloud.org:group_fedora-iot:fedora-iot -y "$@"
  sudo dnf copr disable -y @fedora-iot/fedora-iot
  sudo dnf copr remove -y @fedora-iot/fedora-iot
}

install_from_compose(){
  source /etc/os-release
  case "${ID}-${VERSION_ID}" in
    fedora-rawhide)
      compose_host="http://kojipkgs.fedoraproject.org"
      compose_id="latest-Fedora-${VERSION_ID^}"
      compose_streams="${COMPOSE_STREAMS:-Everything}"
      compose_base_url="${COMPOSE_BASE_URL:-${compose_host}/compose/${VERSION_ID}/${compose_id}/compose}"
      ;;
    fedora-*)
      compose_host="http://kojipkgs.fedoraproject.org"
      compose_streams="${COMPOSE_STREAMS:-Everything}"
      compose_base_url="${COMPOSE_BASE_URL:-${compose_host}/compose/updates/f${VERSION_ID}-updates/compose}"
      ;;
    centos-*)
      compose_host="https://composes.stream.centos.org"
      compose_id="latest-CentOS-Stream"
      compose_streams="${COMPOSE_STREAMS:-BaseOS AppStream}"
      compose_base_url="${COMPOSE_BASE_URL:-${compose_host}/stream-${VERSION_ID}/production/${compose_id}/compose}"
      ;;
    rhel-*)
      compose_base_url="${COMPOSE_BASE_URL:-}"
      [ -n "${compose_base_url}" ] || log_error "Compose base URL must be set for RHEL (eg='http://download.host/.../latest-RHEL-Compose/compose/')"
      compose_streams="${COMPOSE_STREAMS:-BaseOS AppStream}"
      [ -n "${compose_streams}" ] || log_error "Streams must be set for RHEL (default='BaseOS AppStream')"
      ;;
    *)
      log_error "OS not supported"
      ;;
  esac
  for stream in ${compose_streams}; do
    repo_name="compose-${ID}-${VERSION_ID}-${stream}"
    sudo tee "/etc/yum.repos.d/compose-${repo_name}.repo" <<EOF
[${repo_name}]
name=${repo_name}
baseurl=${compose_base_url}/${stream}/$(uname -m)/os/
enabled=1
gpgcheck=0
EOF
  done
  sudo dnf install --disablerepo=* --enablerepo=compose* -y "$@"
  sudo rm -f /etc/yum.repos.d/compose-*
}


install_rpms_from() {
  local install_source="${1}"
  [ -n "$install_source" ] || log_error "Installation source must be provided as first argument"
  shift
  rpms="${*}"
  case "${install_source}" in
    "distro-repos")
      sudo dnf install -y $rpms
      ;;
    "fedora-iot-copr")
      install_from_copr $rpms
      ;;
    "compose")
      install_from_compose $rpms
      ;;
    *)
      log_error "Unsupported installation source: '${install_source}'"
      ;;
  esac
}

install_client() {
  if [ -v "PACKIT_COPR_RPMS" ]; then
    # if PACKIT_COPR_RPMS is defined and contains the 'go-fdo-client'
    # package it means we are running the tests in the client repo
    if [[ "${PACKIT_COPR_RPMS}" =~ go-fdo-client ]]; then
      log_info "Expected Go FDO Client RPM:"
      for i in ${PACKIT_COPR_RPMS}; do
        echo "    ⚙ $i"
      done | sort
    else
      install_rpms_from "fedora-iot-copr" ${go_fdo_client_rpms}
    fi
  else
    # If PACKIT_COPR_RPMS is not defined it means we are running the test
    # locally
    if  [ -n "${client_installation_source}" ]; then
      install_rpms_from "${client_installation_source}" "${go_fdo_client_rpms}"
    else
      if [ -f "build/package/rpm/go-fdo-client.spec" ] ; then
        commit="$(git rev-parse --short HEAD)"
        rpm -q go-fdo-client| grep -q "go-fdo-client.*git${commit}.*" || {
          make rpm
          sudo dnf install -y rpmbuild/rpms/"$(uname -m)"/*git"${commit}"*.rpm
        }
      else
        install_rpms_from "fedora-iot-copr" "${go_fdo_client_rpms}"
      fi
    fi
  fi
  log_info "Installed Go FDO Client RPM:"
  echo "    ⚙ $(rpm -q ${go_fdo_client_rpms})"
}

uninstall_client() {
  [ -v "PACKIT_COPR_RPMS" ] || sudo dnf remove -y ${go_fdo_client_rpms}
}

install_server() {
  # If PACKIT_COPR_RPMS is defined it means that all the rpms were built and installed already by packit
  if [ -v "PACKIT_COPR_RPMS" ]; then
    if [[ "${PACKIT_COPR_RPMS}" =~ go-fdo-server ]]; then
      log_info "Expected Go FDO Server RPMs:"
      for i in ${PACKIT_COPR_RPMS}; do
        echo "    ⚙ $i"
      done | sort
    else
      install_from_copr ${go_fdo_server_rpms}
    fi
  else
    # If PACKIT_COPR_RPMS is not defined it means we are running the test
    # locally
    if  [ -n "${server_installation_source}" ]; then
      install_rpms_from "${server_installation_source}" "${go_fdo_server_rpms}"
    else
      if [ -f "build/package/rpm/go-fdo-server.spec" ] ; then
        commit="$(git rev-parse --short HEAD)"
        rpm -q go-fdo-server | grep -q "go-fdo-server.*git${commit}.*" || {
          make rpm
          sudo dnf install -y rpmbuild/rpms/"$(uname -m)"/go-fdo-server-[1-9.]*.git"${commit}"*.rpm \
                              rpmbuild/rpms/noarch/go-fdo-server-{manufacturer,owner,rendezvous}-[1-9.]*.git"${commit}"*.rpm

        }
      else
        install_rpms_from "fedora-iot-copr" "${go_fdo_client_rpms}"
      fi
    fi
  fi
  # Make sure the RPMS are installed
  installed_rpms=$(rpm -q --qf "%{nvr}.%{arch} " ${go_fdo_server_rpms})
  log_info "Installed Go FDO Server RPMs:"
  for i in ${installed_rpms}; do
    echo "    ⚙ $i"
  done | sort
  sudo chmod o+rX /etc/pki/go-fdo-server/
}

uninstall_server() {
  [ -v "PACKIT_COPR_RPMS" ] || sudo dnf remove -y ${go_fdo_server_rpms}
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
  get_go_fdo_server_logs "${role}" >"${log_file}"
}

save_service_logs_manufacturer() {
  save_go_fdo_server_logs manufacturer "${manufacturer_log_file}"
}

save_service_logs_rendezvous() {
  save_go_fdo_server_logs rendezvous "${rendezvous_log_file}"
}

save_service_logs_owner() {
  save_go_fdo_server_logs owner "${owner_log_file}"
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

cleanup_home_dirs() {
  for server in rendezvous manufacturer owner; do
    local homedir_var="rpm_${server}_home_dir"
    sudo rm -vrf "${!homedir_var:?}"
  done
}

cleanup_databases() {
  for server in rendezvous manufacturer owner; do
    local dbdir_var="rpm_${server}_database_file"
    if [[ -v "${dbdir_var}" ]]; then
      sudo rm -vf "${!dbdir_var:?}"
    fi
  done
}

cleanup_drop_ins() {
  local reload_systemd=0
  for service in rendezvous manufacturer owner; do
    local drop_in_dir_var="systemd_${service}_drop_in_dir"
    if [[ -d "${!drop_in_dir_var}" ]]; then
      reload_systemd=1
      sudo rm -vrf "${!drop_in_dir_var:?}"
    fi
  done
  if [[ ${reload_systemd} -eq 1 ]]; then
    sudo systemctl daemon-reload
  fi
}

remove_files() {
  log_info "Removing files from '${base_dir:?}'"
  sudo rm -vrf "${base_dir:?}"/*
  log_info "Removing files from '${rpm_certs_dir}'"
  sudo rm -vf "${rpm_certs_dir:?}"/*
  log_info "Removing systemd drop-in files"
  cleanup_drop_ins
  log_info "Removing database files"
  cleanup_databases
  log_info "Removing server home directories"
  cleanup_home_dirs
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
