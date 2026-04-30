#!/bin/bash
# Linux test runner without SSH: run tests in startup script and read serial output.

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TERRAFORM_DIR="$SCRIPT_DIR/gcp/terraform"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

OS_VERSION=""
DB_VERSION="mysql-8.0"
TEST_NUM="all"
KEEP_VM=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --os)    OS_VERSION="$2"; shift 2 ;;
        --db)    DB_VERSION="$2"; shift 2 ;;
        --test)  TEST_NUM="$2";   shift 2 ;;
        --keep-vm) KEEP_VM=true;    shift ;;
        -h|--help)
            echo "Usage: $0 --os <os_version> --db <db_version> [--test 1|2|3|4|all] [--keep-vm]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

[[ -z "$OS_VERSION" ]] && { echo "ERROR: --os is required"; exit 1; }
: "${RELEEM_API_KEY:?RELEEM_API_KEY env var must be set}"
: "${GCP_PROJECT:?GCP_PROJECT env var must be set}"

MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-ReleemRootPw$(date +%s)!}"
GCP_ZONE="${GCP_ZONE:-us-central1-a}"
SSH_PUBLIC_KEY="${SSH_PUBLIC_KEY:-ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCreleemdummy releem-test}"

case "$OS_VERSION" in
    ubuntu-22.04) OS_IMAGE_FAMILY="ubuntu-2204-lts"; OS_IMAGE_PROJECT="ubuntu-os-cloud" ;;
    ubuntu-20.04) OS_IMAGE_FAMILY="ubuntu-2004-lts"; OS_IMAGE_PROJECT="ubuntu-os-cloud" ;;
    debian-12)    OS_IMAGE_FAMILY="debian-12";       OS_IMAGE_PROJECT="debian-cloud" ;;
    debian-11)    OS_IMAGE_FAMILY="debian-11";       OS_IMAGE_PROJECT="debian-cloud" ;;
    rocky-8)      OS_IMAGE_FAMILY="rocky-linux-8";   OS_IMAGE_PROJECT="rocky-linux-cloud" ;;
    centos-7)     OS_IMAGE_FAMILY="centos-7";        OS_IMAGE_PROJECT="centos-cloud" ;;
    *) echo "ERROR: Unknown OS version '$OS_VERSION'"; exit 1 ;;
esac

OS_SLUG="$(echo "$OS_VERSION" | tr '.' '-')"
DB_SLUG="$(echo "$DB_VERSION" | tr '.' '-')"
VM_NAME="releem-test-${OS_SLUG}-${DB_SLUG}"
FW_NAME="${VM_NAME}-allow-ssh"

echo "[INFO] Checking for stale GCP resources..."
if gcloud compute instances describe "$VM_NAME" --zone="$GCP_ZONE" --project="$GCP_PROJECT" &>/dev/null; then
    echo "[INFO] Deleting stale VM: $VM_NAME"
    gcloud compute instances delete "$VM_NAME" --zone="$GCP_ZONE" --project="$GCP_PROJECT" --quiet
fi
if gcloud compute firewall-rules describe "$FW_NAME" --project="$GCP_PROJECT" &>/dev/null; then
    echo "[INFO] Deleting stale firewall: $FW_NAME"
    gcloud compute firewall-rules delete "$FW_NAME" --project="$GCP_PROJECT" --quiet
fi

PAYLOAD_DIR="$(mktemp -d /tmp/releem-linux-payload-XXXX)"
trap 'rm -rf "$PAYLOAD_DIR"' EXIT

cp "$REPO_ROOT/install.sh" "$PAYLOAD_DIR/install.sh"
cp "$REPO_ROOT/mysqlconfigurer.sh" "$PAYLOAD_DIR/mysqlconfigurer.sh"
cp "$SCRIPT_DIR/linux/helpers.sh" "$PAYLOAD_DIR/helpers.sh"
cp "$SCRIPT_DIR/linux/run_all.sh" "$PAYLOAD_DIR/run_all.sh"
cp "$SCRIPT_DIR/linux/test_01_install_auto.sh" "$PAYLOAD_DIR/test_01_install_auto.sh"
cp "$SCRIPT_DIR/linux/test_02_install_existing_user.sh" "$PAYLOAD_DIR/test_02_install_existing_user.sh"
cp "$SCRIPT_DIR/linux/test_03_apply_config.sh" "$PAYLOAD_DIR/test_03_apply_config.sh"
cp "$SCRIPT_DIR/linux/test_04_rollback_config.sh" "$PAYLOAD_DIR/test_04_rollback_config.sh"

PAYLOAD_TGZ="/tmp/releem-linux-tests-${OS_SLUG}-${DB_SLUG}-$$.tar.gz"
tar -czf "$PAYLOAD_TGZ" -C "$PAYLOAD_DIR" .
TEST_PAYLOAD_B64="$(base64 < "$PAYLOAD_TGZ" | tr -d '\n')"
rm -f "$PAYLOAD_TGZ"

TF_WORKSPACE_DIR="$(mktemp -d /tmp/releem-tf-${OS_VERSION}-${DB_VERSION}-XXXX)"
trap 'rm -rf "$TF_WORKSPACE_DIR" "$PAYLOAD_DIR"' EXIT
cp -r "$TERRAFORM_DIR/." "$TF_WORKSPACE_DIR/"

cat > "$TF_WORKSPACE_DIR/terraform.tfvars" <<EOFVARS
gcp_project      = "$GCP_PROJECT"
zone             = "$GCP_ZONE"
os_image_family  = "$OS_IMAGE_FAMILY"
os_image_project = "$OS_IMAGE_PROJECT"
os_version       = "$OS_SLUG"
os_type          = "linux"
db_version       = "$DB_VERSION"
db_root_password = "$MYSQL_ROOT_PASSWORD"
releem_api_key   = "$RELEEM_API_KEY"
test_selection   = "$TEST_NUM"
test_payload_b64 = "$TEST_PAYLOAD_B64"
ssh_public_key   = "$SSH_PUBLIC_KEY"
ssh_user         = "releem-test"
allowed_ssh_cidr = "0.0.0.0/0"
use_spot         = true
EOFVARS

export GOOGLE_OAUTH_ACCESS_TOKEN
GOOGLE_OAUTH_ACCESS_TOKEN=$(gcloud auth print-access-token 2>/dev/null) || {
    echo "[ERROR] Failed to get GCP access token. Run: gcloud auth login"
    exit 1
}

echo "[INFO] Provisioning Linux VM: OS=$OS_VERSION DB=$DB_VERSION test=$TEST_NUM"
pushd "$TF_WORKSPACE_DIR" >/dev/null

TF_CACHE="${TF_CACHE_DIR:-$HOME/.terraform.d/releem-test-cache}"
if [[ -d "$TF_CACHE/.terraform/providers" ]]; then
    cp -f "$TF_CACHE/.terraform.lock.hcl" . 2>/dev/null || true
    terraform init -input=false -no-color -plugin-dir="$TF_CACHE/.terraform/providers" >/dev/null
else
    terraform init -input=false -no-color >/dev/null
    mkdir -p "$TF_CACHE"
    cp -r .terraform "$TF_CACHE/"
    cp -f .terraform.lock.hcl "$TF_CACHE/" 2>/dev/null || true
fi

for _tf_attempt in 1 2 3; do
    if terraform apply -auto-approve -input=false -no-color; then
        break
    fi
    if [[ $_tf_attempt -lt 3 ]]; then
        echo "[INFO] terraform apply failed (attempt $_tf_attempt/3), retrying in 30s..."
        sleep 30
    else
        echo "[ERROR] terraform apply failed after 3 attempts"
        exit 1
    fi
done

VM_IP=$(terraform output -raw vm_external_ip)
HOSTNAME=$(terraform output -raw hostname)
popd >/dev/null

echo "[INFO] VM provisioned: $VM_IP (hostname: $HOSTNAME)"

if [[ "$KEEP_VM" == "false" ]]; then
    trap 'echo "[INFO] Destroying VM..."; GOOGLE_OAUTH_ACCESS_TOKEN=$(gcloud auth print-access-token 2>/dev/null); export GOOGLE_OAUTH_ACCESS_TOKEN; cd "$TF_WORKSPACE_DIR" && terraform destroy -auto-approve -input=false -no-color; rm -rf "$TF_WORKSPACE_DIR" "$PAYLOAD_DIR"' EXIT
fi

echo "[INFO] Waiting for serial result markers (up to 40 min)..."
SERIAL_TIMEOUT=2400
SERIAL_ELAPSED=0
TEST_EXIT=1

while [[ $SERIAL_ELAPSED -lt $SERIAL_TIMEOUT ]]; do
    SERIAL_OUT=$(gcloud compute instances get-serial-port-output "$VM_NAME" --zone="$GCP_ZONE" --project="$GCP_PROJECT" --port=1 2>/dev/null || true)

    if echo "$SERIAL_OUT" | grep -q "RELEEM_TEST_RESULT:PASS"; then
        echo "[SUCCESS] Linux tests passed for OS=$OS_VERSION DB=$DB_VERSION"
        TEST_EXIT=0
        break
    fi
    if echo "$SERIAL_OUT" | grep -q "RELEEM_TEST_RESULT:FAIL"; then
        echo "[FAILURE] Linux tests failed for OS=$OS_VERSION DB=$DB_VERSION"
        TEST_EXIT=1
        break
    fi

    sleep 15
    SERIAL_ELAPSED=$((SERIAL_ELAPSED + 15))
    echo "[INFO] still waiting... ${SERIAL_ELAPSED}s"
done

if [[ $SERIAL_ELAPSED -ge $SERIAL_TIMEOUT ]]; then
    echo "[ERROR] Timed out waiting for Linux test result marker"
    TEST_EXIT=1
fi

LOG_DIR="$SCRIPT_DIR/../test-results"
mkdir -p "$LOG_DIR"
SERIAL_LOG="$LOG_DIR/serial-${OS_VERSION}-${DB_VERSION}-$(date +%Y%m%d-%H%M%S).log"
gcloud compute instances get-serial-port-output "$VM_NAME" --zone="$GCP_ZONE" --project="$GCP_PROJECT" --port=1 > "$SERIAL_LOG" 2>/dev/null || true
echo "[INFO] Serial log saved to $SERIAL_LOG"

exit $TEST_EXIT
