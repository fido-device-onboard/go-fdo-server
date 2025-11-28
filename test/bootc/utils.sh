#! /usr/bin/env bash

# Import util functions
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../rpm/utils.sh"

build_bootc_container() {
  local base_image=$1

  if [[ ${ID} == "fedora" ]]; then
    tee Containerfile > /dev/null << EOF
FROM ${base_image}
RUN cat > /etc/yum.repos.d/fedora-iot.repo << 'REPO_EOF'
[copr:copr.fedorainfracloud.org:group_fedora-iot:fedora-iot]
name=Copr repo for fedora-iot owned by @fedora-iot
baseurl=https://download.copr.fedorainfracloud.org/results/@fedora-iot/fedora-iot/fedora-\$releasever-\$basearch/
type=rpm-md
skip_if_unavailable=True
gpgcheck=1
gpgkey=https://download.copr.fedorainfracloud.org/results/@fedora-iot/fedora-iot/pubkey.gpg
repo_gpgcheck=0
enabled=1
enabled_metadata=1
REPO_EOF
RUN dnf install -y go-fdo-client
EOF
  elif [[ ${ID} == "centos" ]]; then
    tee Containerfile > /dev/null << EOF
FROM ${base_image}
RUN cat > /etc/yum.repos.d/fedora-iot.repo << 'REPO_EOF'
[copr:copr.fedorainfracloud.org:group_fedora-iot:fedora-iot]
name=Copr repo for fedora-iot owned by @fedora-iot
baseurl=https://download.copr.fedorainfracloud.org/results/@fedora-iot/fedora-iot/centos-stream-\$releasever-\$basearch/
type=rpm-md
skip_if_unavailable=True
gpgcheck=1
gpgkey=https://download.copr.fedorainfracloud.org/results/@fedora-iot/fedora-iot/pubkey.gpg
repo_gpgcheck=0
enabled=1
enabled_metadata=1
REPO_EOF
RUN dnf install -y go-fdo-client
EOF
  fi

  podman build --retry=5 --retry-delay=10s -t "fdo-bootc:latest" -f Containerfile .
}

generate_iso_from_bootc() {
  local bootc_image_builder=$1
  rm -fr output
  mkdir -pv output
  sudo podman run \
      --rm \
      -it \
      --privileged \
      --pull=newer \
      --security-opt label=type:unconfined_t \
      -v "$(pwd)/output:/output" \
      -v "/var/lib/containers/storage:/var/lib/containers/storage" \
      "${bootc_image_builder}" \
      --type anaconda-iso \
      --rootfs xfs \
      --use-librepo=true \
      "localhost/fdo-bootc:latest"
}

generate_kickstart_iso() {
    if [[ ! -v "PACKIT_COPR_RPMS" ]]; then
      sudo dnf install -y lorax
    fi
    rm -fr /var/lib/libvirt/images/install.iso
    isomount=$(mktemp -d)
    sudo mount -v -o "ro" "output/bootiso/install.iso" "$isomount"
    new_ks_file="bib.ks"
    cat > "${new_ks_file}" << EOFKS
text
$(cat "${isomount}/osbuild-base.ks")
$(cat "${isomount}/osbuild.ks")
EOFKS
    sed -i '/%include/d' "${new_ks_file}"
    sed -i '/%post --erroronfail/i\
user --name=admin --groups=wheel --homedir=/home/admin --iscrypted --password=\$6\$GRmb7S0p8vsYmXzH\$o0E020S.9JQGaHkszoog4ha4AQVs3sk8q0DvLjSMxoxHBKnB2FBXGQ/OkwZQfW/76ktHd0NX5nls2LPxPuUdl.' "${new_ks_file}"
    sed -i "/%post --erroronfail/i\
sshkey --username admin \"${ssh_key_pub}\"" "${new_ks_file}"
    sed -i "/bootc switch/a\
go-fdo-client --blob /boot/device_credential --debug device-init ${manufacturer_protocol}://${manufacturer_ip}:${manufacturer_port} --device-info=iot-device --key ec256" "${new_ks_file}"
    sed -i '/bootc switch/a\
echo "admin ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/admin' "${new_ks_file}"
    log_info "==== New kickstart file ===="
    cat "${new_ks_file}"
    log_info "============================"
    log_info "Writing new ISO"
    sudo mkksiso -c "console=ttyS0,115200" --ks "$new_ks_file" "output/bootiso/install.iso" "/var/lib/libvirt/images/install.iso"
    sudo umount -v "$isomount"
    rm -rf "$isomount"
}

install_server() {
  # Configure firewall
  configure_firewall

  # Configure libvirt
  configure_libvirt

  if [ ! -v "PACKIT_COPR_RPMS" ]; then
    sudo dnf install -y golang make
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

install_client() {
  local base_image=$1
  local bootc_image_builder=$2

  # Build container and generate ISO
  log_info "Building bootc container with go-fdo-client installed"
  build_bootc_container $base_image

  log_info "Using bootc image builder to generate anaconda-iso"
  generate_iso_from_bootc $bootc_image_builder

  # Create modified kickstart file
  log_info "Generating kickstart file and mkksiso"
  generate_kickstart_iso
}

configure_firewall() {
  if [[ ! -v "PACKIT_COPR_RPMS" ]]; then
    sudo dnf install -y firewalld
  fi
  sudo systemctl start firewalld
}

configure_libvirt() {
    if [[ ! -v "PACKIT_COPR_RPMS" ]]; then
      log_info "Install required packages"
      local packages=(podman jq gobject-introspection qemu-img qemu-kvm libvirt-client libvirt-daemon-kvm libvirt-daemon)
      sudo dnf install -y "${packages[@]}"
    fi

    log_info "Configuring libvirt permissions"
    sudo tee /etc/polkit-1/rules.d/50-libvirt.rules > /dev/null << 'EOF'
polkit.addRule(function(action, subject) {
    if (action.id == "org.libvirt.unix.manage" &&
        subject.isInGroup("adm")) {
            return polkit.Result.YES;
    }
});
EOF

    log_info "Starting libvirt daemon"
    sudo systemctl start libvirtd
    if ! sudo virsh list --all > /dev/null; then
        echo "Failed to connect to libvirt" >&2
        return 1
    fi

    # Setup libvirt network
    log_info "Setting up libvirt network"
    local network_xml="/tmp/integration.xml"
    sudo tee "${network_xml}" > /dev/null << 'EOF'
<network xmlns:dnsmasq='http://libvirt.org/schemas/network/dnsmasq/1.0'>
  <name>integration</name>
  <uuid>1c8fe98c-b53a-4ca4-bbdb-deb0f26b3579</uuid>
  <forward mode='nat'>
    <nat>
      <port start='1024' end='65535'/>
    </nat>
  </forward>
  <bridge name='integration' zone='trusted' stp='on' delay='0'/>
  <mac address='52:54:00:36:46:ef'/>
  <ip address='192.168.100.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.100.2' end='192.168.100.254'/>
      <host mac='34:49:22:B0:83:30' name='vm-1' ip='192.168.100.50'/>
      <host mac='34:49:22:B0:83:31' name='vm-2' ip='192.168.100.51'/>
      <host mac='34:49:22:B0:83:32' name='vm-3' ip='192.168.100.52'/>
    </dhcp>
  </ip>
  <dnsmasq:options>
    <dnsmasq:option value='dhcp-vendorclass=set:efi-http,HTTPClient:Arch:00016'/>
    <dnsmasq:option value='dhcp-option-force=tag:efi-http,60,HTTPClient'/>
    <dnsmasq:option value='dhcp-boot=tag:efi-http,&quot;http://192.168.100.1/httpboot/EFI/BOOT/BOOTX64.EFI&quot;'/>
  </dnsmasq:options>
</network>
EOF

    # Define network if it doesn't exist
    if ! sudo virsh net-info integration > /dev/null 2>&1; then
        sudo virsh net-define "${network_xml}"
    fi

    # Start network if not active
    if [[ $(sudo virsh net-info integration | awk '/Active/ {print $2}') == "no" ]]; then
        sudo virsh net-start integration
    fi
}

run_device_initialization() {
  local os_variant=$1
  local boot_args=$2
  local guest_ip="192.168.100.50"
  if [[ ! -v "PACKIT_COPR_RPMS" ]]; then
    sudo dnf install -y virt-install
  fi
  sudo qemu-img create -f qcow2 "/var/lib/libvirt/images/disk.qcow2" 10G
  sudo restorecon -Rv /var/lib/libvirt/images/
  sudo virt-install --name="fdo-bootc" \
      --disk "path=/var/lib/libvirt/images/disk.qcow2,format=qcow2" \
      --ram 3072 \
      --vcpus 2 \
      --network "network=integration,mac=34:49:22:B0:83:30" \
      --os-type linux \
      --os-variant "${os_variant}" \
      --cdrom "/var/lib/libvirt/images/install.iso" \
      --boot "${boot_args}" \
      --nographics \
      --noautoconsole \
      --wait=-1 \
      --noreboot

  log_info "Starting VM..."
  sudo virsh start "fdo-bootc"

  # Wait for SSH
  if ! wait_for_ssh $guest_ip; then
      return 1
  fi

}

get_voucher_guid() {
  local guid=$(curl --fail --silent "${manufacturer_url}/api/v1/vouchers" | jq -r '.[0].guid')
  echo "${guid}"
}

run_fido_device_onboard() {
  local onboarded=1
  local guest_ip="192.168.100.50"
  log_info "Adding host entries for FDO services in vm"
  sudo ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${guest_ip}" \
    'echo -e "192.168.100.1 manufacturer\n192.168.100.1 rendezvous\n192.168.100.1 owner" | sudo tee -a /etc/hosts > /dev/null'
  log_info "Running FIDO Device Onboard"
  sudo ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${guest_ip}" \
    'set -o pipefail; sudo go-fdo-client --blob /boot/device_credential onboard --key ec256 --kex ECDH256 --debug | tee /tmp/onboarding.log'
  result=$(sudo ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${guest_ip}" 'cat /tmp/onboarding.log')
  if [[ "$result" =~ "FIDO Device Onboard Complete" ]]; then
    onboarded=0
  fi
  return ${onboarded}
}

# Wait for SSH to be available
wait_for_ssh() {
  local ip_address=$1
  local max_attempts=30
  local attempt=0

  log_info "Waiting for SSH on ${ip_address}..."
  while [[ ${attempt} -lt ${max_attempts} ]]; do
      if ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${ip_address}" 'echo -n "READY"' 2>/dev/null | grep -q "READY"; then
          log_success "SSH is ready"
          return 0
      fi
      attempt=$((attempt + 1))
      sleep 10
  done

  log_error "SSH connection timed out after $((max_attempts * 10)) seconds"
  return 1
}

remove_files() {
  # Remove container generated during test
  sudo podman rmi fdo-bootc:latest
  # Destroy and delete virtual machine
  if [[ $(sudo virsh domstate "fdo-bootc") == "running" ]]; then
    sudo virsh destroy "fdo-bootc"
  fi
  sudo virsh undefine "fdo-bootc" --nvram
  # Delete disk files
  sudo rm -fr /var/lib/libvirt/images/disk.qcow2
  # Remove output files generated by bib
  sudo rm -fr output
  # Remove iso file
  sudo rm -fr "/var/lib/libvirt/images/install.iso"
}

cleanup() {
  [ ! -v "PACKIT_COPR_RPMS" ] || save_logs
  stop_services
  unset_hostnames
  uninstall_server
  remove_files
}
