{{/*
Expand the name of the chart.
*/}}
{{- define "aag-connector-slack.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "aag-connector-slack.fullname" -}}
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
Common labels
*/}}
{{- define "aag-connector-slack.labels" -}}
helm.sh/chart: {{ include "aag-connector-slack.name" . }}
{{ include "aag-connector-slack.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "aag-connector-slack.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aag-connector-slack.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
