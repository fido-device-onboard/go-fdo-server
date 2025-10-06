# FDO E2E Testing with TMT

This directory contains the Test Management Tool (TMT) framework implementation for end-to-end testing of go-fdo-server using Fedora RPM packages.

## Structure

```
test/fmf/
├── .fmf/
│   └── version          # FMF format version
├── plans/
│   └── onboarding.fmf   # Test execution plan
└── tests/
    ├── onboarding.fmf   # Device onboarding e2e test
    ├── resale.fmf       # Device resale protocol test
    └── fsim.fmf         # FSIM functionality tests
```

## Test Plan

The `plans/onboarding.fmf` defines:

- **Provision**: Virtual Fedora environment
- **Prepare**: Install go-fdo-server RPM and dependencies
- **Execute**: Run TMT-managed test discovery and execution
- **Discover**: Find and run FMF-formatted tests
- **Finish**: Collect logs and artifacts

## Test Cases

### onboarding.fmf
Tests the complete FDO device onboarding workflow:
- Device initialization
- Manufacturer voucher creation
- Owner voucher transfer
- TO0 protocol execution
- Device onboarding completion

### resale.fmf
Tests the FDO device resale protocol:
- Secondary owner setup
- Ownership transfer
- Device re-onboarding

### rendezvous.fmf
Tests FDO rendezvous server basic functionality:
- Server startup and binding
- Basic connectivity verification
- Process management

## Integration with Packit

The tests are integrated into `.packit.yaml` with:

```yaml
- job: tests
  trigger: pull_request
  identifier: e2e-fedora
  fmf_path: test/fmf
  tmt_plan: plans/onboarding
  packages: [go-fdo-server-fedora]
  targets:
    - fedora-latest-stable
    - fedora-latest
    - fedora-rawhide
```

## Running Tests

Tests run automatically on pull requests via Testing Farm. They can also be run locally with:

```bash
# Install TMT
sudo dnf install tmt

# Run the test plan
tmt run --all plans/onboarding

# Run specific test
tmt run --all discover --how fmf --test /tests/onboarding
```

## Requirements

- go-fdo-server RPM package
- openssl
- systemd (for service management)
- curl, wget (for FSIM tests)