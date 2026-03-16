terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.gcp_project
  zone    = var.zone
}

locals {
  vm_name  = "releem-test-${var.os_version}-${replace(var.db_version, ".", "-")}"
  hostname = "releem-agent-test-${var.os_version}"

  template_vars = {
    hostname         = local.hostname
    os_version       = var.os_version
    db_version       = var.db_version
    db_root_password = var.db_root_password
    releem_api_key   = var.releem_api_key
    test_selection   = var.test_selection
    test_payload_b64 = var.test_payload_b64
    ssh_public_key   = var.ssh_public_key
  }

  vm_metadata = var.os_type == "linux" ? {
    ssh-keys       = "${var.ssh_user}:${var.ssh_public_key}"
    startup-script = templatefile("${path.module}/startup/linux_bootstrap.sh", local.template_vars)
    } : {
    ssh-keys                   = "Administrator:${var.ssh_public_key}"
    windows-startup-script-ps1 = templatefile("${path.module}/startup/windows_bootstrap.ps1", local.template_vars)
  }
}

data "google_compute_image" "os_image" {
  family  = var.os_image_family
  project = var.os_image_project
}

resource "google_compute_firewall" "allow_ssh_test" {
  name    = "${local.vm_name}-allow-ssh"
  network = var.network

  allow {
    protocol = "tcp"
    ports    = ["22", "5985", "5986"]
  }

  source_ranges = [var.allowed_ssh_cidr]
  target_tags   = ["releem-test"]
}

resource "google_compute_instance" "test_vm" {
  name         = local.vm_name
  machine_type = var.machine_type
  zone         = var.zone

  depends_on = [google_compute_firewall.allow_ssh_test]

  tags = ["releem-test"]

  boot_disk {
    initialize_params {
      image = data.google_compute_image.os_image.self_link
      size  = var.os_type == "windows" ? 60 : 30
      type  = "pd-standard"
    }
  }

  network_interface {
    network = var.network
    access_config {}
  }

  scheduling {
    preemptible         = var.use_spot
    automatic_restart   = var.use_spot ? false : true
    on_host_maintenance = var.use_spot ? "TERMINATE" : "MIGRATE"
    provisioning_model  = var.use_spot ? "SPOT" : "STANDARD"
  }

  metadata = local.vm_metadata

  service_account {
    scopes = ["cloud-platform"]
  }
}
