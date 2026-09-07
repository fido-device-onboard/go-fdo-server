
# FIDO Device Onboarding CI

## Prerequisites:

* make
* golang
* docker: https://docs.docker.com/engine/install/
* docker compose: https://docs.docker.com/compose/install/
* act: https://github.com/nektos/act
* tmt: https://docs.fedoraproject.org/en-US/ci/tmt/#_install

## CI Workflows and jobs

The list of available workflows and the corresponding jobs can be listed with `act -l`:
```bash
➜ act -l
INFO[0000] Using docker host 'unix:///var/run/docker.sock', and daemon socket 'unix:///var/run/docker.sock' 
Stage  Job ID                            Job name                     Workflow name           Workflow file   Events                    
0      check-spelling                    check spelling               Code scanning           analysis.yml    push,pull_request,schedule
0      commitlint                        check commitlint             Code scanning           analysis.yml    push,pull_request,schedule
0      analysis_devskim                  check devskim                Code scanning           analysis.yml    pull_request,schedule,push
0      test-rpms                         Test srpm and rpm builds     Continuous integration  ci.yml          push,pull_request         
0      test-onboarding                   Test FIDO device onboarding  Continuous integration  ci.yml          push,pull_request         
0      test-resale                       Test FIDO resale protocol    Continuous integration  ci.yml          push,pull_request         
0      test-fsim-wget                    Test FSIM wget               Continuous integration  ci.yml          push,pull_request         
0      test-fsim-upload                  Test FSIM upload             Continuous integration  ci.yml          push,pull_request         
0      test-fsim-download                Test FSIM download           Continuous integration  ci.yml          push,pull_request         
0      test-container-fsim-fdo-wget      Test FSIM fdo.wget           Container Tests         containers.yml  push,pull_request         
0      test-container-onboarding         Test FIDO device onboarding  Container Tests         containers.yml  push,pull_request         
0      test-container-resale             Test FIDO resale protocol    Container Tests         containers.yml  pull_request,push         
0      test-container-fsim-fdo-upload    Test FSIM fdo.upload         Container Tests         containers.yml  push,pull_request         
0      test-container-fsim-fdo-download  Test FSIM fdo.download       Container Tests         containers.yml  push,pull_request       
```

## Testing the CI jobs locally with `act`:

When running the workflow jobs it's important to bind mount the `./test/workdir` dir:
```bash
➜ act --container-options "-v ${PWD}/test/workdir:${PWD}/test/workdir" -j test-onboarding
```

## Testing the CI jobs with `tmt`:

The list of available tmt tests can be listed with `tmt test ls`:
```bash
➜ tmt test ls
```

When running the tmt tests it's important to be verbose `-vvv` to see the actual test's output:
```bash
/test/fmf/tests/test-onboarding
➜ tmt -vvv run test --name /test/fmf/tests/test-onboarding
```

## Testing the CI jobs locally without `act` or `tmt`:

It's also possible to run the scripts directly without `act` or `tmt`.
Any script from `./test/{ci,container,fmf}` directories can be executed from the shell:
*  CI tests
```bash
➜ ./test/ci/test-onboarding.sh
```
* Container tests
```bash
➜ ./test/container/test-onboarding.sh
```
* TMT tests
```bash
➜ ./test/fmf/tests/test-onboarding.sh
```
* Debugging
```bash
➜ sh -x ./test/ci/test-onboarding.sh
# or
➜ sh -x ./test/container/test-onboarding.sh
# or
➜ sh -x ./test/fmf/tests/test-onboarding.sh
```

## RPM Tests

RPM tests deploy the FDO servers using RPM packages. These tests support multiple installation sources controlled by environment variables.

### Installation Sources

The RPM tests support the following installation sources:

1. **distro-repos** - Install from standard distribution repositories
2. **fedora-iot-copr** - Install from the fedora-iot COPR repository
3. **compose** - Install from a specific compose URL

### Environment Variables

Control which installation source to use with these environment variables:

- `INSTALLATION_SOURCE` - Sets the installation source for both client and server
- `CLIENT_INSTALLATION_SOURCE` - Override installation source for the client only
- `SERVER_INSTALLATION_SOURCE` - Override installation source for the server only

### Running RPM Tests with Different Installation Sources

#### Using distribution repositories
```bash
➜ INSTALLATION_SOURCE=distro-repos ./test/rpm/test-onboarding.sh
```

#### Using fedora-iot COPR repository
```bash
➜ INSTALLATION_SOURCE=fedora-iot-copr ./test/rpm/test-onboarding.sh
```

#### Using a compose

##### Fedora and CentOS Stream (automatic compose URL detection)
For Fedora and CentOS Stream, the compose URL is automatically detected based on the OS version:

```bash
➜ INSTALLATION_SOURCE=compose ./test/rpm/test-onboarding.sh
```

##### RHEL (requires explicit compose URL)
For RHEL, you must specify the compose base URL:

```bash
➜ INSTALLATION_SOURCE=compose \
  COMPOSE_BASE_URL="http://download.host/.../latest-RHEL-Compose/compose/" \
  ./test/rpm/test-onboarding.sh
```

##### Custom compose URL and streams
You can override the compose URL and streams for any distribution:

```bash
# Custom compose URL
➜ INSTALLATION_SOURCE=compose \
  COMPOSE_BASE_URL="http://custom.host/compose/path/" \
  ./test/rpm/test-onboarding.sh

# Custom streams (default: "Everything" for Fedora, "BaseOS AppStream" for CentOS/RHEL)
➜ INSTALLATION_SOURCE=compose \
  COMPOSE_STREAMS="BaseOS AppStream" \
  ./test/rpm/test-onboarding.sh

# Both custom URL and streams
➜ INSTALLATION_SOURCE=compose \
  COMPOSE_BASE_URL="http://custom.host/compose/path/" \
  COMPOSE_STREAMS="BaseOS AppStream CRB" \
  ./test/rpm/test-onboarding.sh
```

### Running All RPM Tests

You can run any RPM test from the `test/rpm/` directory:

```bash
# List available RPM tests
➜ ls test/rpm/test-*.sh

# Run specific tests
➜ INSTALLATION_SOURCE=compose ./test/rpm/test-device-ca-api.sh
➜ INSTALLATION_SOURCE=compose ./test/rpm/test-fsim-wget.sh
➜ INSTALLATION_SOURCE=compose ./test/rpm/test-resale.sh
```

### Default Behavior (without INSTALLATION_SOURCE)

When `INSTALLATION_SOURCE` is not set:
- If running in a development environment with spec files present, RPMs are built locally from the current git commit
- Otherwise, packages are installed from the fedora-iot COPR repository
