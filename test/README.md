
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

When running the workflow jobs it's important to bind mount the `/tmp` dir:
```bash
➜ act --container-options "-v /tmp:/tmp" -j test-onboarding
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
