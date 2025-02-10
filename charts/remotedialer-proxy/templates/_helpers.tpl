{{/*
API Extension Name - To be used in other variables
*/}}
{{- define "api-extension.name" }}
{{- default "api-extension" .Values.apiExtensionName }}
{{- end}}

{{/*
Expand the name of the chart.
*/}}
{{- define "remotedialer-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "remotedialer-proxy.fullname" -}}
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
{{- define "remotedialer-proxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "remotedialer-proxy.labels" -}}
helm.sh/chart: {{ include "remotedialer-proxy.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{ include "remotedialer-proxy.selectorLabels" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "remotedialer-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "remotedialer-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: api-extension
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "remotedialer-proxy.serviceAccountName" -}}
{{- default (printf "%s-sa" (include "api-extension.name" .)) .Values.serviceAccount.name }}
{{- end }}


{{/*
Namespace to use
*/}}
{{- define "remotedialer-proxy.namespace" -}}
{{- default "cattle-system" .Values.namespaceOverride }}
{{- end }}

{{/*
Role to use
*/}}
{{- define "remotedialer-proxy.role" -}}
{{- default (printf "%s-role" (include "api-extension.name" .)) .Values.roleOverride }}
{{- end }}
