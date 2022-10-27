package kubernetes

import _ "embed"

//go:embed secrets-upcloud-csi.yaml
var SecretsTemplate string

//go:embed crd-upcloud-csi.yaml
var CRDTemplate string

//go:embed rbac-upcloud-csi.yaml
var RbacTemplate string

//go:embed setup-upcloud-csi.yaml
var CSITemplate string

//go:embed snapshot-webhook-upcloud-csi.yaml
var SnapshotTemplate string
