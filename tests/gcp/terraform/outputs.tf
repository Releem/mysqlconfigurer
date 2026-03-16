output "vm_external_ip" {
  description = "External IP address of the test VM"
  value       = google_compute_instance.test_vm.network_interface[0].access_config[0].nat_ip
}

output "vm_name" {
  description = "Name of the test VM"
  value       = google_compute_instance.test_vm.name
}

output "hostname" {
  description = "Releem agent hostname configured on the VM"
  value       = local.hostname
}

output "ssh_command" {
  description = "SSH command to connect to the test VM"
  value       = "ssh -i <your_private_key> ${var.ssh_user}@${google_compute_instance.test_vm.network_interface[0].access_config[0].nat_ip}"
}
