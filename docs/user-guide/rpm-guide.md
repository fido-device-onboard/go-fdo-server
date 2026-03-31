# FDO Server RPM Packages

This document shows how to build, install and run the FDO Servers using RPM packaging tools.

**Note**: This document assumes that the platform supports RPM package management tooling using `dnf`.

## Building the RPM Packages

The top level Makefile can be used to build the FDO server RPM packages:

```bash
$ make rpm
```

This will install any necessary dependencies and build the server RPM packages. The packages can be found in the `rpmbuild` subdirectory.

## Installing the FDO Manufacturing server

After the FDO server RPMs are built the Manufacturing server can be installed. Install both the go-fdo-server-manufacturer and go-fdo-server RPM packages. Example:

```bash
# dnf install ./rpmbuild/rpms/noarch/go-fdo-server-manufacturer-1.0.0.el10.noarch.rpm ./rpmbuild/rpms/x86_64/go-fdo-server-1.0.0.el10.x86_64.rpm
```

The packages install a number of files, creates the go-fdo-server group and go-fdo-server-manufacturer user, and installs a systemd service for running the Manufacturing server. Check if the Manufacturing server's systemd unit file is available:

```bash
# systemctl list-unit-files | grep go-fdo-server-manufacturer
go-fdo-server-manufacturer.service disabled disabled
```

## Running the FDO Manufacturing server

Prior to running the Manufacturing server ensure that all prerequisites have been satisfied and a configuration file is present. Refer to the [User Guide](README.md) for more information.

The server's HTTP endpoint needs to be accessible for both server management and device access. To ensure that the server's HTTP endpoint is not blocked by the system firewall, open the server's network port. Example:

```bash
# firewall-cmd --add-port=8038/tcp --permanent
# systemctl restart firewalld
```

Enable and start the Manufacturing server via the systemd service:

```bash
# systemctl enable --now go-fdo-server-manufacturer.service
```

Verify the server is active:

```bash
# systemctl status go-fdo-server-manufacturer.service
● go-fdo-server-manufacturer.service - Go FDO manufacturer server
     Loaded: loaded (/usr/lib/systemd/system/go-fdo-server-manufacturer.service; enabled; preset: disabled)
     Active: active (running) since Fri 2026-03-20 16:15:47 EDT; 38s ago
...
Mar 20 16:15:48 localhost go-fdo-server[4901]: [16:15:48] INFO: Database initialized successfully
Mar 20 16:15:48 localhost go-fdo-server[4901]:   type: sqlite
Mar 20 16:15:48 localhost go-fdo-server[4901]: [16:15:48] INFO: Listening
Mar 20 16:15:48 localhost go-fdo-server[4901]:   local: [::]:8038
```

Optional: verify the server's network endpoint is functioning and accessible. This can be done by probing the server's `/health` API endpoint using a tool such as curl. Example:

```bash
$ curl http://192.168.124.132:8038/health
{"message":"the service is up and running","status":"OK","version":"1.0.0"}
```

## Installing the FDO Rendezvous server

After the FDO server RPMs are built the Rendezvous server can be installed. Install both the go-fdo-server-rendezvous and go-fdo-server RPM packages. Example:

```bash
# dnf install ./rpmbuild/rpms/noarch/go-fdo-server-rendezvous-1.0.0.el10.noarch.rpm ./rpmbuild/rpms/x86_64/go-fdo-server-1.0.0.el10.x86_64.rpm
```

The packages install a number of files, creates the go-fdo-server group and go-fdo-server-rendezvous user, and installs a systemd service for running the Rendezvous server. Check if the Rendezvous server's systemd unit file is available:

```bash
# systemctl list-unit-files | grep go-fdo-server-rendezvous
go-fdo-server-rendezvous.service disabled disabled
```

## Running the FDO Rendezvous server

Prior to running the Rendezvous server ensure that all prerequisites have been satisfied and a configuration file is present. Refer to the [User Guide](README.md) for more information.

The server's HTTP endpoint needs to be accessible for server management, device and Owner server access. To ensure that the server's HTTP endpoint is not blocked by the system firewall, open the server's network port. Example:

```bash
# firewall-cmd --add-port=8041/tcp --permanent
# systemctl restart firewalld
```

Enable and start the Rendezvous server via the systemd service:

```bash
# systemctl enable --now go-fdo-server-rendezvous.service
```

Verify the server is active:

```bash
# systemctl status go-fdo-server-rendezvous.service
● go-fdo-server-rendezvous.service - Go FDO Rendezvous server
     Loaded: loaded (/usr/lib/systemd/system/go-fdo-server-rendezvous.service; enabled; preset: disabled)
     Active: active (running) since Fri 2026-03-20 17:21:34 EDT; 16h ago
...
Mar 20 17:21:34 localhost go-fdo-server[5160]: [17:21:34] INFO: Database initialized successfully
Mar 20 17:21:34 localhost go-fdo-server[5160]:   type: sqlite
Mar 20 17:21:34 localhost go-fdo-server[5160]: [17:21:34] INFO: Listening
Mar 20 17:21:34 localhost go-fdo-server[5160]:   local: [::]:8041
```

Optional: verify the server's network endpoint is functioning and accessible. This can be done by probing the server's `/health` API endpoint using a tool such as curl. Example:

```bash
$ curl http://192.168.124.132:8041/health
{"message":"the service is up and running","status":"OK","version":"1.0.0"}
```

## Installing the FDO Owner server

After the FDO server RPMs are built the Owner server can be installed. Install both the go-fdo-server-owner and go-fdo-server RPM packages. Example:

```bash
# dnf install ./rpmbuild/rpms/noarch/go-fdo-server-owner-1.0.0.el10.noarch.rpm ./rpmbuild/rpms/x86_64/go-fdo-server-1.0.0.el10.x86_64.rpm
```

The packages install a number of files, creates the go-fdo-server group and go-fdo-server-owner user, and installs a systemd service for running the Owner server. Check if the Owner server's systemd unit file is available:

```bash
# systemctl list-unit-files | grep go-fdo-server-owner
go-fdo-server-owner.service disabled disabled
```

## Running the FDO Owner server

Prior to running the Owner server ensure that all prerequisites have been satisfied and a configuration file is present. Refer to the [User Guide](README.md) for more information.

The server's HTTP endpoint needs to be accessible for server management and access by the device during onboarding. To ensure that the server's HTTP endpoint is not blocked by the system firewall, open the server's network port. Example:

```bash
# firewall-cmd --add-port=8043/tcp --permanent
# systemctl restart firewalld
```

Enable and start the Owner server via the systemd service:

```bash
# systemctl enable --now go-fdo-server-owner.service
```

Verify the server is active:

```bash
# systemctl status go-fdo-server-owner.service
● go-fdo-server-owner.service - Go FDO owner server
     Loaded: loaded (/usr/lib/systemd/system/go-fdo-server-owner.service; enabled; preset: disabled)
     Active: active (running) since Sun 2026-03-22 16:35:04 EDT; 18s ago
...
Mar 22 16:35:05 localhost go-fdo-server[9194]: [16:35:05] INFO: Database initialized successfully
Mar 22 16:35:05 localhost go-fdo-server[9194]:   type: sqlite
Mar 22 16:35:05 localhost go-fdo-server[9194]: [16:35:05] INFO: Listening
Mar 22 16:35:05 localhost go-fdo-server[9194]:   local: [::]:8043
```

Optional: verify the server's network endpoint is functioning and accessible. This can be done by probing the server's `/health` API endpoint using a tool such as curl. Example:

```bash
$ curl http://192.168.124.132:8043/health
{"message":"the service is up and running","status":"OK","version":"1.0.0"}
```