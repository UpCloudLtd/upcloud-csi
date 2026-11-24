{{/*
Image reference for a map with repository and tag keys.
*/}}
{{- define "upcloud-csi-driver.image" -}}
{{- printf "%s:%s" .repository .tag -}}
{{- end -}}

{{/*
Zone shared by the controller and node plugins.
*/}}
{{- define "upcloud-csi-driver.zone" -}}
{{- $message := "zone is required: specify the UpCloud zone (e.g. fi-hel1)" -}}
{{- $zone := required $message .Values.zone -}}
{{- if not (kindIs "string" $zone) -}}
{{- fail "zone must be a string specifying the UpCloud zone (e.g. fi-hel1)" -}}
{{- end -}}
{{- if not (trim $zone) -}}
{{- fail $message -}}
{{- end -}}
{{- $zone -}}
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
