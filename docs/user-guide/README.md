# FIDO Device Onboarding Users Guide

## Introduction to FIDO Device Onboarding

**Note**: This document provides an overview of the FIDO Device Onboarding process, not a detailed reference.  For a detailed understanding of the FDO process, refer to the [FIDO Device Onboard Specification](https://fidoalliance.org/specifications/download-iot-specifications/) available from FIDO Alliance.

FIDO Device Onboarding (FDO) is an automated onboarding mechanism to securely integrate new devices into your IoT infrastructure. FDO verifies device authenticity and protects the device configuration process, synchronizing each new device with your existing infrastructure upon installation.

With FDO, you can benefit from the following:

* A zero-touch procedure for enrolling (*onboarding*) a new device to the owner’s management platform at scale.
* The device’s authenticity and ownership are cryptographically verified prior to the exchange of sensitive information.
* Tooling for device provisioning across untrusted networks via a securely encrypted communications channel.
* Ownership credentials and configuration are applied during device onboarding rather than at device manufacture (late binding).

FDO is a two-phase process:

* The first phase is *Device Initialization* and is performed by the device manufacturer. During this phase, the device is set up to perform onboarding after it has been purchased. On completion of this phase:
  * A set of credentials has been stored on the device. These device credentials are used to prove the identity of the device.
  * An *Ownership Voucher* has been created. The Ownership Voucher is a digital document that is used to prove the ownership chain of the device.
  * The device is assigned a Globally Unique Identifier (GUID).
* The second phase is *Transfer of Ownership* and occurs after the device has been purchased. It is during the *Transfer of Ownership* that the device is onboarded to the new owner’s infrastructure.

### Components of the FDO system

FDO onboarding employs a device-side client application and three specialized FDO servers: *Manufacturing*, *Rendezvous*, and *Owner*.

**Device client**  
This is installed on the device and performs the following operations:

1. During device manufacture the client application is used to initialize the device’s FDO state with the help of the Manufacturing server.
2. When the device is installed by the final owner, the client application performs the onboarding process in concert with the Rendezvous and Owner FDO servers.

The `go-fdo-client` application is a Golang implementation of a device client. It is available from the [Go FDO Client project](https://github.com/fido-device-onboard/go-fdo-client).

**Manufacturing server**  
Deployed at the device manufacturer’s site, this server is responsible for the *FDO Device Initialization* protocol, which prepares the device for the FDO onboarding process and creates the Ownership Voucher. Device initialization is initiated by the device client. Once device initialization completes, the Manufacturing server has no further involvement with the onboarding process.

**Rendezvous server**  
To enable late binding of device ownership - required in those cases where the device’s ownership is not known at the time the device is manufactured - there needs to be a discovery service which can be used by the device at the start of onboarding to securely identify its owner. The Rendezvous server provides this discovery service. The device client contacts the Rendezvous server at the start of the onboarding process to retrieve the Owner server’s network address(es).

**Owner server**  
The Owner server is deployed by the device owner’s organization. It is responsible for integrating the new device into the owner’s IoT infrastructure. When the device is installed and first booted at the owner’s facility, the device client connects to the Owner server and initiates the final phase of the onboarding process. The FDO onboarding process completes once the Owner server has finished provisioning the device for use.

The `go-fdo-server` application is a Golang implementation of all three of the above services. The FDO service it provides - Manufacturing, Rendezvous, or Owner - is set through configuration. It is available from the [Go FDO Server project](https://github.com/fido-device-onboard/go-fdo-server).

### The FDO workflow

1. Device Initialization (FDO DI protocol)
   1. The device client contacts the Manufacturing server.
   2. The client and server generate credentials which are stored on the device by the client.
   3. The Manufacturing server produces the Ownership Voucher containing the network address of the Rendezvous server.
   4. The device is assigned a GUID.
   5. Device Initialization completes and the device is ready for onboarding at the owner’s facility.

2. On-site Onboarding (FDO protocols Transfer of Ownership (TO0), (TO1), (TO2))
   1. Prior to first boot of the device, the Ownership Voucher is uploaded to the Owner server by the server administrator.
   2. The upload causes the Owner server to extract the Rendezvous server’s network address from the Ownership Voucher.
   3. The Owner server contacts the Rendezvous server and uses the Ownership Voucher to prove it is the legitimate owner of the corresponding device.
   4. The Owner server provides the Rendezvous server with its network address(es) which the device client uses to contact the Owner server.
   5. The Rendezvous server stores the Owner server information for eventual use by the device client.
   6. On first boot the device client retrieves the Rendezvous server network address from its on-board device credentials and contacts the Rendezvous server.
   7. The Rendezvous server looks up the device’s Ownership Voucher and associated owner network address(es) and returns them to the device client.
   8. The device client disconnects from the Rendezvous server and attempts to connect to the Owner server using the supplied network address(es).
   9. The device client and Owner server establish mutual trust by authenticating each other using information from the Ownership Voucher and device credentials.
   10. The device client and Owner server establish a securely encrypted communications channel.
   11. The Owner server provisions the device over the secure channel by exchanging ServiceInfo key/value data.
   12. The device client uses the ServiceInfo data to implement the desired device configuration.
   13. On completion of the configuration process, the device is operational and the FDO workflow ends.

### FDO ServiceInfo Modules (FSIMs)

FDO gives the Owner server the ability to perform administrative operations on the device during onboarding using *FDO ServiceInfo Modules (FSIMs)*. FSIMs can be used to download files to the device, upload files from the device, and execute commands on the device. FSIMs execute after mutual trust has been established and the secure communications channel is active.

See the [FSIM Guide](fsim-guide.md) for a detailed description of the available FSIMs and the [Server Configuration Reference](server-config.md) for FSIM configuration details.

## Server Installation

This section explains how to install and run the FDO servers.

### Prerequisites

#### Required FDO Credentials

The FDO process requires cryptographic credentials (certificates and private keys) in order to meet its security guarantees. These certificates must be provided by the party that is deploying the FDO infrastructure. They are not provided by this project.

The certificates and private keys required for FDO protocol operation:

* **Manufacturer Certificate/Key** is issued by the manufacturer. It is required by the Manufacturing server and is used during device initialization.
* **Owner Certificate/Key** is issued by the owner organization. The private key is required by the Owner server. The public certificate is provided to the Manufacturing server for use when extending the Ownership Voucher.
* **Device CA Certificate/Key** is issued by the manufacturer to certify that the device is legitimate. Both the public certificate and private key are used by the Manufacturing server during device initialization. The public certificate is provided to the Owner server for verifying the device’s identity.

Refer to the [Certificate Setup Guide](certificates.md) for more information.

#### HTTPS/TLS Certificates

All FDO servers support network access via HTTP. Servers can be configured to enable HTTPS for security purposes. In this case, additional TLS certificates for server authentication need to be provided. These certificates are separate from the certificates required by FDO. Refer to the [Certificate Setup Guide](certificates.md) and the [server configuration documentation](server-config.md) for additional information.

#### Database support

All FDO servers require a database in order to persist state across restarts. The FDO servers can be configured to use either SQLite or PostgreSQL as the database implementation. Refer to the [server configuration documentation](server-config.md) for additional information.

### Container Installation Guide

For installing and running the FDO servers using containers refer to the [Dockerfile Usage Guide](dockerfile-usage.md).

### RPM Installation Guide

For installing and running the FDO servers using the RPM packages refer to the [RPM Guide](rpm-guide.md).

## Server Configuration and Management

The `go-fdo-server` configuration is partitioned into two domains:
* the base configuration, which is provided via a configuration file/CLI, and
* the operational configuration, which is managed via a REST API.

The server's base configuration includes required settings such as the server's network address and database configuration. It must be provided when the server starts either via a configuration file or command line flags. It is static: it cannot be modified at run time.

The server's operational configuration includes onboarding-related data, such as Ownership Vouchers and device certificates. It is provided at run time and persisted to the server's database.

### Configuration File

The `go-fdo-server` base configuration must be provided in order to start the server. While most of the base configuration can be provided via the CLI the full configuration can only be set using a configuration file. See the [Server Configuration Reference](server-config.md) for a description of the configuration file, including example configuration files for each server role (Manufacturing, Rendezvous, and Owner).

### Command Line Interface
#### Manufacturing Server CLI
*TBD*
#### Rendezvous Server CLI
*TBD*
#### Owner Server CLI
*TBD*

### Server Management API
#### Manufacturing Server API
*TBD*
#### Rendezvous Server API
*TBD*
#### Owner Server API
*TBD*

## Basic Onboarding Example
See the [Quickstart Guide](quick-start.md) for a complete onboarding example.

## RV-Bypass
*TBD*

## Resale Protocol
*TBD*

## Troubleshooting
*TBD*
