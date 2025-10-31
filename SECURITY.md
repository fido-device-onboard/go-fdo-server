# Security Policy

## Supported Versions

Information about different FIDO Device Onboard (FDO) release versions can be found on the [FDO Release page](https://fido-device-onboard.github.io/docs-fidoiot/latest/releases/).

## Authentication and Access Control

### Management API Security

**Important**: The FDO server management APIs (`/api/v1/` endpoints) do not include built-in authentication mechanisms. These APIs provide administrative access to:

- Rendezvous information management
- Ownership voucher management
- Device redirect configuration
- TO0 protocol triggers

### Recommended Security Measures

For production deployments, implement access control for management APIs using one of these approaches:

#### 1. Reverse Proxy with Basic Authentication

Deploy nginx or Apache as a reverse proxy with HTTP Basic Authentication. See [REVERSE_PROXY.md](REVERSE_PROXY.md) for detailed configuration instructions.

#### 2. Network-Level Protection

- Deploy FDO servers in a private network segment
- Use firewall rules to restrict access to management API ports (typically 8038, 8043)
- Access via VPN or bastion hosts only

### Protocol vs Management Separation

The FDO protocol endpoints (`/fdo/101/msg/`) must remain accessible for legitimate device communication and should not be protected by authentication. Only the management APIs require access control.

## Reporting a Vulnerability

Instructions for reporting a vulnerability can be found on the [FDO Reporting Issues page](https://wiki.lfedge.org/display/FDO/Reporting+Issues).
