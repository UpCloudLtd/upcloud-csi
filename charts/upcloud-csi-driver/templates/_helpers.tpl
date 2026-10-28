{{/*
Image reference for a map with repository and tag keys.
*/}}
{{- define "upcloud-csi-driver.image" -}}
{{- printf "%s:%s" .repository .tag -}}
{{- end -}}

{{/*
Labels used to identify chart-managed objects. Selectors keep the original app labels.
*/}}
{{- define "upcloud-csi-driver.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}
