#!/bin/bash
set -euox pipefail

# Logging functions
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# Import util functions
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)/../../ci/utils.sh"

# Configuration
TEST_UUID=$(uuidgen)
GUEST_IP="192.168.100.50"
SSH_OPTIONS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5)
SSH_KEY="id_rsa"
sudo ssh-keygen -f "${SSH_KEY}" -N "" -q -t rsa-sha2-256 -b 2048
SSH_KEY_PUB=$(cat "${SSH_KEY}.pub")
manufacturer_ip="192.168.100.1"
rendezvous_ip="192.168.100.1"
owner_ip="192.168.100.1"

source /etc/os-release
log_info "Detected OS: ${ID} ${VERSION_ID}"

case "${ID}-${VERSION_ID}" in
    "fedora-43")
        OS_VARIANT="fedora-unknown"
        BASE_IMAGE_URL="quay.io/fedora/fedora-bootc:43"
        BIB_URL="quay.io/centos-bootc/bootc-image-builder:latest"
        BOOT_ARGS="uefi"
        ;;
    "fedora-44")
        OS_VARIANT="fedora-rawhide"
        BASE_IMAGE_URL="quay.io/fedora/fedora-bootc:44"
        BIB_URL="quay.io/centos-bootc/bootc-image-builder:latest"
        BOOT_ARGS="uefi"
        ;;
    "centos-9")
        OS_VARIANT="centos-stream9"
        BASE_IMAGE_URL="quay.io/centos-bootc/centos-bootc:stream9"
        BIB_URL="quay.io/centos-bootc/bootc-image-builder:latest"
        BOOT_ARGS="uefi,firmware.feature0.name=secure-boot,firmware.feature0.enabled=no"
        ;;
    "centos-10")
        OS_VARIANT="centos-stream9"
        BASE_IMAGE_URL="quay.io/centos-bootc/centos-bootc:stream10"
        BIB_URL="quay.io/centos-bootc/bootc-image-builder:latest"
        BOOT_ARGS="uefi,firmware.feature0.name=secure-boot,firmware.feature0.enabled=no"
        ;;
    *)
        log_error "Unsupported distro: ${ID}-${VERSION_ID}"
        exit 1
        ;;
esac

prepare_env() {
    log_info "Starting CI environment setup"

    local packages=(podman jq qemu-img qemu-kvm libvirt-client libvirt-daemon-kvm libvirt-daemon virt-install ansible-core firewalld lorax gobject-introspection)
    dnf install -y "${packages[@]}"

    systemctl start firewalld
    
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

    log_success "CI environment setup completed successfully"
}

# Generate new kickstart file and make new iso file
generate_kickstart_iso() {
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
sshkey --username admin \"${SSH_KEY_PUB}\"" "${new_ks_file}"
    sed -i "/bootc switch/a\
go-fdo-client --blob /boot/device_credential --debug device-init http://${manufacturer_ip}:8038 --device-info=iot-device --key ec256" "${new_ks_file}"
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

# Wait for SSH to be available
wait_for_ssh() {
    local ip_address=$1
    local max_attempts=30
    local attempt=0
    
    log_info "Waiting for SSH on ${ip_address}..."
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        if ssh "${SSH_OPTIONS[@]}" -i "${SSH_KEY}" "admin@${ip_address}" 'echo -n "READY"' 2>/dev/null | grep -q "READY"; then
            log_success "SSH is ready"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 10
    done
    
    log_error "SSH connection timed out after $((max_attempts * 10)) seconds"
    return 1
}

# Prepare test environment
prepare_env

# Infrastructure setup
log_info "Creating directories"
create_directories

log_info "Generating service certificates"
generate_service_certs

log_info "Adding host entries for FDO services in host machine"
echo -e "${manufacturer_ip} manufacturer\n${rendezvous_ip} rendezvous\n${owner_ip} owner" | sudo tee -a /etc/hosts > /dev/null

log_info "Build and install 'go-fdo-server' binary"
pushd ../../.. > /dev/null
install_server
popd > /dev/null

log_info "Start services"
start_services

log_info "Wait for the services to be ready"
wait_for_services_ready

log_info "Setting or updating Rendezvous Info (RendezvousInfo)"
set_or_update_rendezvous_info "${manufacturer_url}" "${rendezvous_service_name}" "${rendezvous_dns}" "${rendezvous_port}"

# Build container and generate ISO
log_info "Building bootc container with go-fdo-client installed"

tee Containerfile > /dev/null << EOF
FROM ${BASE_IMAGE_URL}
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

podman build --retry=5 --retry-delay=10s -t "fdo-bootc:${TEST_UUID}" -f Containerfile .

log_info "Using bootc image builder to generate anaconda-iso"
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
    "${BIB_URL}" \
    --type anaconda-iso \
    --rootfs xfs \
    --use-librepo=true \
    "localhost/fdo-bootc:${TEST_UUID}"

# Create modified kickstart file
log_info "Generating kickstart file and mkksiso"
generate_kickstart_iso

# VM management
log_info "Provisioning VM..."
sudo qemu-img create -f qcow2 "/var/lib/libvirt/images/disk-${TEST_UUID}.qcow2" 10G
sudo restorecon -Rv /var/lib/libvirt/images/
sudo virt-install --name="fdo-bootc-${TEST_UUID}" \
    --disk "path=/var/lib/libvirt/images/disk-${TEST_UUID}.qcow2,format=qcow2" \
    --ram 3072 \
    --vcpus 2 \
    --network "network=integration,mac=34:49:22:B0:83:30" \
    --os-type linux \
    --os-variant "${OS_VARIANT}" \
    --cdrom "/var/lib/libvirt/images/install.iso" \
    --boot "${BOOT_ARGS}" \
    --nographics \
    --noautoconsole \
    --wait=-1 \
    --noreboot

log_info "Starting VM..."
sudo virsh start "fdo-bootc-${TEST_UUID}"

# Wait for SSH
if ! wait_for_ssh "${GUEST_IP}"; then
    exit 1
fi

# FDO onboarding process
log_info "Get device initialization voucher guid"
guid=$(curl --fail --silent "${manufacturer_url}/api/v1/vouchers" | jq -r '.[0].guid')
echo "Device initialized with GUID: ${guid}"

log_info "Adding host entries for FDO services in vm"
sudo ssh "${SSH_OPTIONS[@]}" -i "${SSH_KEY}" "admin@${GUEST_IP}" \
    'echo -e "192.168.100.1 manufacturer\n192.168.100.1 rendezvous\n192.168.100.1 owner" | sudo tee -a /etc/hosts > /dev/null'

log_info "Sending Ownership Voucher to the Owner"
send_manufacturer_ov_to_owner "${manufacturer_url}" "${guid}" "${owner_url}"

log_info "Setting or updating Owner Redirect Info (RVTO2Addr)"
set_or_update_owner_redirect_info "${owner_url}" "${owner_service_name}" "${owner_dns}" "${owner_port}"

sleep 60

log_info "Running FIDO Device Onboard"
sudo ssh "${SSH_OPTIONS[@]}" -i "${SSH_KEY}" "admin@${GUEST_IP}" \
    'set -o pipefail; sudo go-fdo-client --blob /boot/device_credential onboard --key ec256 --kex ECDH256 --debug | tee /tmp/onboarding.log'

log_info "Check Device Onboard result"
result=$(sudo ssh "${SSH_OPTIONS[@]}" -i "${SSH_KEY}" "admin@${GUEST_IP}" 'cat /tmp/onboarding.log')
if [[ ! "$result" =~ "FIDO Device Onboard Complete" ]]; then
    log_error "Failed to onboard"
    exit 1
fi

log_success "FIDO Device Onboard completed successfully"
exit 0
