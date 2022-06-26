{{/*
Expand the name of the chart.
*/}}
{{- define "truenas-scale-csi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "truenas-scale-csi.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "truenas-scale-csi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "truenas-scale-csi.labels" -}}
helm.sh/chart: {{ include "truenas-scale-csi.chart" . }}
{{ include "truenas-scale-csi.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "truenas-scale-csi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "truenas-scale-csi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "truenas-scale-csi.serviceAccountName" -}}
{{- default (include "truenas-scale-csi.fullname" .) .Values.serviceAccount.name }}
{{- end }}

{{/*
Create the name of the CSI driver
*/}}
{{- define "truenas-scale-csi.csiDriverName" -}}
{{- if eq .Values.settings.type "nfs" }}
{{ .Values.nfsCSIDriverName }}
{{- else }}
{{ .Values.iscsiCSIDriverName }}
{{- end }}
{{- end }}

{{/*
Create the name of the Storage class
*/}}
{{- define "truenas-scale-csi.storageClassName" -}}
{{- if eq .Values.settings.type "nfs" }}
{{ .Values.storageClass.namePrefix }}nfs
{{- else }}
{{ .Values.storageClass.namePrefix }}iscsi
{{- end }}
{{- end }}

{{/*
Create the name of the controller deployment to use
*/}}
{{- define "truenas-scale-csi.controllerDeploymentName" -}}
{{- printf "%s-controller" (include "qnap-csi.fullname" .) }}
{{- end }}

{{/*
Create the name of the node daemonset to use
*/}}
{{- define "truenas-scale-csi.nodeDaemonsetName" -}}
{{- printf "%s-node" (include "qnap-csi.fullname" .) }}
{{- end }}

{{/*
Log level
*/}}
{{- define "truenas-scale-csi.logLevel" -}}
{{- if .Values.settings.enableDebugLogging }}
debug
{{- else }}
info
{{- end }}
{{- end }}
