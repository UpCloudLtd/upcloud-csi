type      = "csi"
plugin_id = "csi-upcloud"
id        = "my-volume"
name      = "My persistent volume"

capability {
  access_mode     = "single-node-writer"
  attachment_mode = "file-system"
}