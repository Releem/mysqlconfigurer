variable "gcp_project" {
  description = "GCP project ID"
  type        = string
}

variable "zone" {
  description = "GCP zone for the test VM"
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "GCP machine type"
  type        = string
  default     = "e2-medium"
}

variable "os_image_family" {
  description = "GCP OS image family (e.g. ubuntu-2204-lts, debian-12, rocky-linux-8, centos-7)"
  type        = string
}

variable "os_image_project" {
  description = "GCP project hosting the OS image (e.g. ubuntu-os-cloud, debian-cloud, rocky-linux-cloud, centos-cloud, windows-cloud)"
  type        = string
}

variable "os_version" {
  description = "Short OS version label used in hostnames and resource names (e.g. ubuntu-22-04, debian-12)"
  type        = string
}

variable "db_version" {
  description = "Database version to install: mysql-8.0 | mysql-8.4 | mariadb-10"
  type        = string
  default     = "mysql-8.0"

  validation {
    condition     = contains(["mysql-8.0", "mysql-8.4", "mariadb-10"], var.db_version)
    error_message = "db_version must be one of: mysql-8.0, mysql-8.4, mariadb-10"
  }
}

variable "db_root_password" {
  description = "MySQL root password to set during bootstrap"
  type        = string
  sensitive   = true
}

variable "releem_api_key" {
  description = "Releem API key passed into VM startup tests"
  type        = string
  sensitive   = true
}

variable "test_selection" {
  description = "Which test to run: 1|2|3|4|all"
  type        = string
  default     = "all"
}

variable "test_payload_b64" {
  description = "Base64 encoded test payload archive (tar.gz for linux, zip for windows)"
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "SSH public key to inject into the VM for access"
  type        = string
  default     = ""
}

variable "ssh_user" {
  description = "SSH user for VM access"
  type        = string
  default     = "releem-test"
}

variable "allowed_ssh_cidr" {
  description = "CIDR range allowed to SSH into the test VM (restrict to your IP)"
  type        = string
  default     = "0.0.0.0/0"
}

variable "network" {
  description = "GCP VPC network name"
  type        = string
  default     = "default"
}

variable "os_type" {
  description = "OS type: linux or windows"
  type        = string
  default     = "linux"

  validation {
    condition     = contains(["linux", "windows"], var.os_type)
    error_message = "os_type must be 'linux' or 'windows'"
  }
}

variable "use_spot" {
  description = "Use spot/preemptible VM to save costs (not recommended for Windows due to long bootstrap)"
  type        = bool
  default     = true
}
