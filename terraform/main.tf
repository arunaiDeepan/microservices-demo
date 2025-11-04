# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Definition of local variables
locals {
  base_apis = [
    "container.googleapis.com",
    "monitoring.googleapis.com",
    "cloudtrace.googleapis.com",
    "cloudprofiler.googleapis.com"
  ]
  memorystore_apis = ["redis.googleapis.com"]
  cluster_name     = google_container_cluster.my_cluster.name
}

# Enable Google Cloud APIs
module "enable_google_apis" {
  source  = "terraform-google-modules/project-factory/google//modules/project_services"
  version = "~> 18.0"

  project_id                  = var.gcp_project_id
  disable_services_on_destroy = false

  # activate_apis is the set of base_apis and the APIs required by user-configured deployment options
  activate_apis = concat(local.base_apis, var.memorystore ? local.memorystore_apis : [])
}


resource "google_compute_network" "vpc" {
  name                    = "${var.name}-vpc"
  auto_create_subnetworks = false
}

# Subnet
resource "google_compute_subnetwork" "subnet" {
  name          = "${var.name}-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = var.region
  network       = google_compute_network.vpc.id

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = "10.1.0.0/16"
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = "10.2.0.0/16"
  }
}

resource "google_container_cluster" "my_cluster" {
  name     = var.name
  location = var.region

  # Remove default node pool
  remove_default_node_pool = true
  initial_node_count       = 1

  node_config {
    disk_type    = "pd-standard"
    disk_size_gb = 20 # Keep it small
  }  

  network    = google_compute_network.vpc.name
  subnetwork = google_compute_subnetwork.subnet.name

  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  workload_identity_config {
    workload_pool = "${var.gcp_project_id}.svc.id.goog"
  }

  addons_config {
    http_load_balancing {
      disabled = false
    }
  }

  deletion_protection = false
}

# Separately Managed Node Pool
resource "google_container_node_pool" "my_cluster_nodes" {
  name       = "${var.name}-node-pool"
  location   = var.region
  cluster    = google_container_cluster.my_cluster.name
  node_count = 1


  node_config {
    preemptible  = false
    machine_type = "n2d-standard-2"
    disk_type    = "pd-standard"
    disk_size_gb = 20

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform"
    ]

    labels = {
      env = "dev"
    }

    workload_metadata_config {
      mode = "GKE_METADATA"
    }
  }

  autoscaling {
    min_node_count = 1
    max_node_count = 2
  }
}

# # Create GKE cluster
# resource "google_container_cluster" "my_cluster" {

#   name     = var.name
#   location = var.region

#   # Enable autopilot for this cluster
#   enable_autopilot = true

#   # Set an empty ip_allocation_policy to allow autopilot cluster to spin up correctly
#   ip_allocation_policy {
#   }

#   # Avoid setting deletion_protection to false
#   # until you're ready (and certain you want) to destroy the cluster.
#   deletion_protection = false

#   depends_on = [
#     module.enable_google_apis
#   ]
# }

# Get credentials for cluster
# module "gcloud" {
#   source  = "terraform-google-modules/gcloud/google"
#   version = "~> 4.0"

#   platform              = "linux"
#   additional_components = ["kubectl", "beta"]

#   create_cmd_entrypoint = "gcloud"
#   # Module does not support explicit dependency
#   # Enforce implicit dependency through use of local variable
#   create_cmd_body = "container clusters get-credentials ${local.cluster_name} --zone=${var.region} --project=${var.gcp_project_id}"
# }

# # Apply YAML kubernetes-manifest configurations
# resource "null_resource" "apply_deployment" {
#   provisioner "local-exec" {
#     interpreter = ["bash", "-exc"]
#     command     = "kubectl apply -k ${var.filepath_manifest} -n ${var.namespace}"
#   }

#   depends_on = [
#     module.gcloud
#   ]
# }

# # Wait condition for all Pods to be ready before finishing
# resource "null_resource" "wait_conditions" {
#   provisioner "local-exec" {
#     interpreter = ["bash", "-exc"]
#     command     = <<-EOT
#     kubectl wait --for=condition=AVAILABLE apiservice/v1beta1.metrics.k8s.io --timeout=180s
#     kubectl wait --for=condition=ready pods --all -n ${var.namespace} --timeout=280s
#     EOT
#   }

#   depends_on = [
#     resource.null_resource.apply_deployment
#   ]
# }
