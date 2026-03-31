# FDO ServiceInfo Modules (FSIMs)

The goal of the onboarding process is to provision the device for use in the device owner's infrastructure. Typically this requires performing tasks such as downloading credentials, modifying configurations, running scripts and other administrative operations.

FDO gives the Owner server the ability to perform these types of administrative operations using *FDO ServiceInfo Modules (FSIMs)*. FSIMs can be used to download files to the device, upload files from the device, and execute commands on the device.

FSIMs execute during onboarding after mutual trust has been established and the secure communications channel is active. This ensures that FSIM operations can perform privileged actions on the device in a secure manner.

**Note**: The process running the `go-fdo-client` must have the necessary privileges to perform these functions on behalf of the Owner server. For example, when downloading files to the device's filesystem, the `go-fdo-client` process must have permission to create and write the file to the destination directory.

To use FSIMs, they must be specified in the Owner server's configuration file. The configuration file supports an ordered list of FSIM configuration entries. FSIMs are performed in the listed order during onboarding. Refer to the Owner Server Configuration section of the [Server Configuration Reference](server-config.md) for configuration details.

The following four FSIMs are supported by the Owner server:

## `fdo.download`

The `fdo.download` FSIM can be used to download a file from the Owner server's filesystem to the device's filesystem. The file is downloaded over the secure communications channel, guaranteeing that sensitive content is protected.

## `fdo.upload`

The `fdo.upload` FSIM can be used to upload a file from the device's filesystem to the Owner server's filesystem. The file is uploaded over the secure communications channel, guaranteeing that sensitive content is protected. The file is saved to a directory that is unique to the device. Refer to the [Server Configuration Reference](server-config.md) for details.

## `fdo.wget`

Like the `fdo.download` FSIM, the `fdo.wget` FSIM downloads a file to the device's filesystem. Unlike the `fdo.download` FSIM, the source of the file is an HTTP server rather than the Owner server itself. The advantage of the `fdo.wget` FSIM is that it has much faster download performance than `fdo.download`, making it a better choice for downloading large files (such as container images).

The `fdo.wget` FSIM does **not** download the file over the secure FDO communications channel. The device connects directly to the target HTTP server. The FDO protocol does not guarantee the security of the HTTP connection. If the file contains sensitive information, it should either be encrypted before downloading or transferred using a properly authenticated HTTPS connection.

## `fdo.command`

The `fdo.command` FSIM can be used by the Owner server to execute programs on the device. These programs are executed by the `go-fdo-client` device client and must be present on the device prior to issuing the `fdo.command`. The `fdo.command` supports passing arguments to the program, and redirecting `stdout` and `stderr` back to the Owner server's logs. The `fdo.command` supports the inlining of shell scripts directly into the configuration file.
