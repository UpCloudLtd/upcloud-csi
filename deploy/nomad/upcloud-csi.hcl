variable "upcloud_username" {
  type = string
}

variable "upcloud_password" {
  type = string
}

variable "upcloud_zone" {
  type = string
}

job "plugin-upcloud-csi" {
  datacenters = [var.upcloud_zone]

  # system job ensures all nodes in the DC have a copy
  type = "system"

  # only one plugin of a given type and ID should be deployed on any given client node
  constraint {
    operator = "distinct_hosts"
    value    = true
  }

  group "nodes" {
    task "plugin" {
      driver = "docker"
      config {
        image = "ghcr.io/upcloudltd/upcloud-csi:latest"
        args = [
          "--endpoint=unix:///csi/csi.sock",
          "--nodehost=${attr.unique.hostname}",
          "--username=${var.upcloud_username}",
          "--password=${var.upcloud_password}",
          "--log-level=info",
        ]
        privileged = true
      }
      csi_plugin {
        id        = "csi-upcloud"
        type      = "monolith"
        mount_dir = "/csi"
      }
    }
  }
}
