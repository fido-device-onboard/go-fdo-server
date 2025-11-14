#!/usr/bin/env bash
set -euox pipefail

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }

# Package management
install_packages() {
    log_info "Installing required packages"
    
    packages=(
        make golang podman jq qemu-img httpd firewalld 
        qemu-kvm libvirt-client libvirt-daemon-kvm 
        libvirt-daemon virt-install ansible-core 
        cargo lorax lsof openssh
    )
    
    dnf install -y "${packages[@]}"
}

# Function to configure services
configure_services() {
    log_info "Configuring services"
    
    # Enable and start services
    sudo systemctl enable --now httpd.service
    sudo systemctl enable --now firewalld
    
    # Configure libvirt permissions
    log_info "Configuring libvirt permissions"
    sudo tee /etc/polkit-1/rules.d/50-libvirt.rules > /dev/null << 'EOF'
polkit.addRule(function(action, subject) {
    if (action.id == "org.libvirt.unix.manage" &&
        subject.isInGroup("adm")) {
            return polkit.Result.YES;
    }
});
EOF
    
    # Start libvirtd
    log_info "Starting libvirt daemon"
    sudo systemctl start libvirtd
    
    # Verify libvirt is working
    if ! sudo virsh list --all > /dev/null; then
        echo "Failed to connect to libvirt" >&2
        return 1
    fi
}

# Function to setup libvirt network
setup_libvirt_network() {
    local network_xml="/tmp/integration.xml"
    
    log_info "Setting up libvirt network"
    
    # Create network configuration
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

log_info "Starting CI environment setup"

# Get OS data
source /etc/os-release
log_info "Detected OS: ${ID} ${VERSION_ID}"

# Install required packages
install_packages

# Configure services
configure_services

# Setup libvirt network
setup_libvirt_network

log_success "CI environment setup completed successfully"
