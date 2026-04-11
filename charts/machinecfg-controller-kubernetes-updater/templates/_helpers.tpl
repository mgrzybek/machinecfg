{{/*
Expand the name of the chart.
*/}}
{{- define "machinecfg-controller-kubernetes-updater.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "machinecfg-controller-kubernetes-updater.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "machinecfg-controller-kubernetes-updater.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "machinecfg-controller-kubernetes-updater.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "machinecfg-controller-kubernetes-updater.selectorLabels" -}}
app.kubernetes.io/name: {{ include "machinecfg-controller-kubernetes-updater.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the Secret holding the NetBox token.
Returns existingSecret when provided, otherwise the chart-managed secret name.
*/}}
{{- define "machinecfg-controller-kubernetes-updater.secretName" -}}
{{- .Values.netbox.existingSecret | default "kubernetes-updater-secret" }}
{{- end }}
