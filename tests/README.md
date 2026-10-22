# Releem Agent Installation Tests

End-to-end tests for `install.sh`, `releem.sh` (Linux) and `windows/install.ps1`, `windows/releem.ps1` (Windows).

Tests run on GCP spot (preemptible) VMs provisioned by Terraform, torn down after each run.

## Linux Test Workflows

Run offline installer, apply and release-publication checks without cloud access:

```bash
bats tests/install.bats tests/mysqlconfigurer.bats tests/release.bats tests/docker.bats tests/docker_entrypoint.bats
```

`tests/docker.bats` checks that the Docker entrypoint starts its command after
both successful and failed initial configuration downloads, preserving the
command's exit status. It uses temporary paths and does not require Docker.

`tests/release.bats` executes the upload block extracted from the existing
`.github/workflows/go.yml` with a mocked AWS CLI. It checks root `v2/` destinations,
public ACLs, content types, and the current glob upload order. In that order,
`current_version_agent` uploads first; publication can expose a partial update
if a later upload fails. Each injected upload failure must stop publication,
preserve the failed and remaining files, and retain the files already uploaded.
These checks do not upload to S3 or introduce a different release mechanism.

| # | Name | Description |
|---|------|-------------|
| 1 | Fresh install (auto user) | Installs agent; script creates the `releem` MySQL user automatically |
| 2 | Install with existing user | `releem` MySQL user is pre-created; install.sh uses provided credentials |
| 3 | Apply configuration | Applies the API-recommended MySQL configuration via `releem.sh -s automatic` |
| 4 | Rollback configuration | Rolls back the applied config via `releem.sh -r` |

Tests 3 and 4 depend on test 1 having run first. The run_all scripts handle this automatically.

## Windows Test Workflows

| # | Name | Description |
|---|------|-------------|
| 1 | Fresh install (auto user) | Installs agent; script creates the `releem` MySQL user automatically |
| 2 | Install with existing user | `releem` MySQL user is pre-created; install.ps1 uses provided credentials |
| 3 | Apply configuration | Applies the API-recommended MySQL configuration |
| 4 | Rollback configuration | Rolls back the applied config |
| 5 | Update delegation | Verifies update flow delegation |
| 6 | Reinstall existing installation | Reinstalls over an existing agent installation |
| 7 | Apply without restart | Applies configuration without restarting MySQL; removes the backup after successful copying without restart |
| 8 | Queue apply | Verifies queued apply behavior |
| 9 | Reinstall rewrites config without prompt | Reinstalls without interactive credential prompts |
| 10 | Install with prompted root password | Installs without `RELEEM_MYSQL_ROOT_PASSWORD` and supplies the root password interactively |

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

```bash
cd tests

export RELEEM_API_KEY="4170dfb9-d55f-4de5-bcc9-555f9187ce98"
export GCP_PROJECT="your-gcp-project-id"
export MYSQL_ROOT_PASSWORD="SomeSecurePassword123!"

# Run all Windows tests on Windows Server 2022 + MySQL 8.0
./run_tests_windows.sh --db mysql-8.0

# Run only test 10, which validates the prompted root password install flow
./run_tests_windows.sh --db mysql-8.0 --test 10

# Keep the VM alive after tests (for debugging)
./run_tests_windows.sh --db mysql-8.0 --test 10 --keep-vm
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
├── run_tests_windows.sh                 # Local Windows test orchestrator
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
    ├── test_05_update_delegation.ps1
    ├── test_06_reinstall_existing_install.ps1
    ├── test_07_apply_without_restart.ps1
    ├── test_08_queue_apply.ps1
    ├── test_09_reinstall_rewrites_config_without_prompt.ps1
    ├── test_10_install_prompt_root_password.ps1
    └── run_all.ps1                      # Run all Windows tests
```

### Windows configurer checks

Run all offline scenarios with PowerShell 7 on Linux or PowerShell on Windows:

```powershell
pwsh -NoProfile -File tests/windows/test_configurer.ps1
pwsh -NoProfile -File tests/windows/test_apply_prepared_unit.ps1
```

Use `-Check ExitCodes`, `-Check Legacy`, `-Check Update`, or
`-Check InstallServices` to run one group. The release workflow (`go.yml`)
runs both scripts with PowerShell 7 before building.
No database, service changes, administrator rights, or Pester are required.
The suite checks:

- Installer detection of MySQL 9.0, 9.7, 26.7 and existing MySQL/MariaDB service names, including restart command generation.
- Directory/include exit codes, path escaping and configuration round trips.
- Legacy aliases, switch values, arguments with spaces and exit-code forwarding.
- Successful updates of legacy-only and outdated-script installations.
- Fresh installation and update failures when either required script cannot download.
- Linux downloads stage inside `RELEEM_WORKDIR` to retain its SELinux labels on installation; reinstall clears old contents before downloading.
- Download or validation failures preserve installed Linux files during updates. Failed fresh installs and reinstalls leave an empty `conf` directory and remove staged downloads.
- A failed configuration-generation request after a Linux update produces a warning while the completed update succeeds.
- Failed Windows file replacement restores the previous installed files, including removal of newly introduced scripts, then restarts a previously running agent service only after restoration succeeds.
- Required files are downloaded and validated before stopping or starting the agent.
- Ordinary manual and automatic apply use the Go-downloaded configuration without requesting another download.
- Initial apply downloads fresh initial configuration first; download failure preserves the live configuration.
- Missing downloaded configurations fail before changing live configuration files.
- Empty configuration responses preserve the existing staging file.
- Linux and Windows back up the current live file before each apply, replacing any existing backup. Successful apply (including copying without restart) or rollback removes the backup; failures leave it available for recovery. Retrying an apply backs up the current file again, even if a previous attempt failed.
- Full apply and managed rollback force restarts when required.
- Manual apply retries the restart for identical staging/live files when a backup exists from a failed attempt. Without a backup, identical manual applies can skip restarting. After successful apply, rollback removes the Releem configuration because no backup remains.

Update scenarios execute the real installer branch with download/service mocks,
then run both installed entry points through the actual help and validation
paths. Only the privilege bootstrap and Windows paths are adapted offline.

Windows Test 5 runs this shared suite before checking delegated `-Update` on a
VM. It verifies both installed files against the payload and runs both names.
Its mocked version is always newer so a release bump cannot skip the update.
