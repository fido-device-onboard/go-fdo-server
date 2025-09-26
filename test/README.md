
# FIDO Device Onboarding CI

## Prerequisites:

* make
* golang
* docker: https://docs.docker.com/engine/install/
* docker compose: https://docs.docker.com/compose/install/
* act: https://github.com/nektos/act

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

## Testing the CI jobs locally without `act`:

It's also possible to run the scripts directly without `act`.
Any script from `./test/{ci,container}` directories can be executed from the shell:
*  CI tests 
```bash
➜ ./test/ci/test-onboarding.sh
```
* Container tests
```bash
➜ ./test/container/test-onboarding.sh
```
* Debugging
```bash
➜ sh -x ./test/ci/test-onboarding.sh
# or
➜ sh -x ./test/container/test-onboarding.sh
```
