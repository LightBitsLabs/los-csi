{{/* Generate basic labels */}}
{{- define "mychart.labels" }}
generator: helm
date: {{ now | htmlDate }}
chart: {{ .Chart.Name }}
version: {{ .Chart.Version }}
{{- end }}

{{/*
Create a default fully qualified driver name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
if a driverNamePrefix is provided we will use that prefix
else we will name the driver with the <namespace>.csi.lightbitslabs.com unless namespace==kube-system
in which case we will name it csi.lightbitslabs.com
*/}}
{{- define "driver.fullname" -}}
{{- if .Values.driverNamePrefix -}}
{{- printf "%s.csi.lightbitslabs.com" .Values.driverNamePrefix | trunc 63 | trimSuffix "." -}}
{{- else -}}
{{- if eq .Release.Namespace "kube-system" -}}
{{- "csi.lightbitslabs.com" -}}
{{- else -}}
{{- printf "%s.csi.lightbitslabs.com" .Release.Namespace | trunc 63 | trimSuffix "." -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create a LB_CSI_NODE_ID
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
returns $(KUBE_NODE_NAME).node.{{ .Release.Namespace }} unless namespace==kube-system
in which case we will name it $(KUBE_NODE_NAME).node
*/}}
{{- define "driver.nodeid" -}}
{{- if eq .Release.Namespace "kube-system" -}}
{{- "$(KUBE_NODE_NAME).node" -}}
{{- else -}}
{{- printf "$(KUBE_NODE_NAME).node.%s" .Release.Namespace | trunc 63 | trimSuffix "." -}}
{{- end -}}
{{- end -}}
{{- include "driver.nodeid" . }}