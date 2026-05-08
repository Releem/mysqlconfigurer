# Releem Agent Installation Tests

End-to-end tests for `install.sh`, `mysqlconfigurer.sh` (Linux) and `windows/install.ps1`, `windows/mysqlconfigurer.ps1` (Windows).

Tests run on GCP spot (preemptible) VMs provisioned by Terraform, torn down after each run.

## Test Workflows

| # | Name | Description |
|---|------|-------------|
| 1 | Fresh install (auto user) | Installs agent; script creates the `releem` MySQL user automatically |
| 2 | Install with existing user | `releem` MySQL user is pre-created; install.sh uses provided credentials |
| 3 | Apply configuration | Applies the API-recommended MySQL configuration via `mysqlconfigurer.sh -s automatic` |
| 4 | Rollback configuration | Rolls back the applied config via `mysqlconfigurer.sh -r` |

Tests 3 and 4 depend on test 1 having run first. The run_all scripts handle this automatically.

## Supported Matrices

**Linux OS**: ubuntu-22.04, ubuntu-20.04, debian-12, debian-11, rocky-8, centos-7
**Windows OS**: windows-server-2022
**DB versions**: mysql-8.0, mysql-8.4, mariadb-10

## Prerequisites

- `terraform` >= 1.5 (https://developer.hashicorp.com/terraform/install)
- `gcloud` CLI authenticated: `gcloud auth application-default login`
- GCP project with Compute Engine API enabled
- SSH key pair (auto-generated if absent at `~/.ssh/releem_test_rsa`)

## Running Locally

### Single OS/DB combination

```bash
cd tests

export RELEEM_API_KEY="4170dfb9-d55f-4de5-bcc9-555f9187ce98"
export GCP_PROJECT="your-gcp-project-id"
export MYSQL_ROOT_PASSWORD="SomeSecurePassword123!"

# Run all 4 tests on Ubuntu 22.04 + MySQL 8.0
./run_tests_local.sh --os ubuntu-22.04 --db mysql-8.0

# Run only test 1
./run_tests_local.sh --os ubuntu-22.04 --db mysql-8.0 --test 1

# Keep the VM alive after tests (for debugging)
./run_tests_local.sh --os ubuntu-22.04 --db mysql-8.0 --keep-vm
```

### Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `RELEEM_API_KEY` | Yes | - | Releem test API key |
| `GCP_PROJECT` | Yes | - | GCP project ID |
| `MYSQL_ROOT_PASSWORD` | No | Auto-generated | MySQL root password for bootstrap |
| `GCP_ZONE` | No | `us-central1-a` | GCP zone |
| `SSH_KEY_PATH` | No | `~/.ssh/releem_test_rsa` | SSH private key path |
| `ALLOWED_SSH_CIDR` | No | Auto-detected public IP | CIDR allowed to SSH to test VM |

### Running Windows tests locally

```powershell
# Windows test runner script (TODO: implement run_tests_windows.sh)
# For now, manually provision a GCP Windows Server 2022 VM,
# copy tests/windows/ and windows/*.ps1 scripts, and run:
.\run_all.ps1 -Test all
```

## Running in GitHub Actions

Trigger the `Test Releem Agent Installation` workflow from the Actions tab:

1. Go to **Actions** → **Test Releem Agent Installation**
2. Click **Run workflow**
3. Select OS version, DB version, and test number (or leave as `all`)

### Required GitHub Secrets

| Secret | Description |
|---|---|
| `RELEEM_TEST_API_KEY` | Releem test API key (`4170dfb9-d55f-4de5-bcc9-555f9187ce98`) |
| `GCP_PROJECT_ID` | GCP project ID |
| `GCP_SA_KEY` | GCP service account JSON with Compute Engine access |
| `RELEEM_TEST_MYSQL_ROOT_PASSWORD` | MySQL root password for test VMs |

### GCP Service Account Permissions

The service account needs:
- `compute.instances.create/delete/get/list`
- `compute.firewalls.create/delete`
- `compute.disks.create/delete`
- `compute.networks.get`

Or simply: `roles/compute.instanceAdmin.v1`

## Directory Structure

```
tests/
├── README.md                            # This file
├── run_tests_local.sh                   # Local Linux test orchestrator
├── gcp/terraform/
│   ├── main.tf                          # GCP VM Terraform definition
│   ├── variables.tf                     # Input variables
│   ├── outputs.tf                       # VM IP, hostname, etc.
│   └── startup/
│       ├── linux_bootstrap.sh           # Installs MySQL + world DB on Linux
│       └── windows_bootstrap.ps1        # Installs MySQL + world DB on Windows
├── linux/
│   ├── helpers.sh                       # Assert functions, logging, API checks
│   ├── test_01_install_auto.sh
│   ├── test_02_install_existing_user.sh
│   ├── test_03_apply_config.sh
│   ├── test_04_rollback_config.sh
│   └── run_all.sh                       # Run all Linux tests
└── windows/
    ├── helpers.ps1
    ├── test_01_install_auto.ps1
    ├── test_02_install_existing_user.ps1
    ├── test_03_apply_config.ps1
    ├── test_04_rollback_config.ps1
    └── run_all.ps1                      # Run all Windows tests
```
