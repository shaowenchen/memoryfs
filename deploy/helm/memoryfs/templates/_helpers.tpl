{{/*
Expand the name of the chart.
*/}}
{{- define "memoryfs.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "memoryfs.fullname" -}}
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

{{- define "memoryfs.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "memoryfs.labels" -}}
helm.sh/chart: {{ include "memoryfs.chart" . }}
app.kubernetes.io/name: {{ include "memoryfs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "memoryfs.selectorLabels" -}}
app.kubernetes.io/name: {{ include "memoryfs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
component: node
{{- end }}

{{- define "memoryfs.headless" -}}
{{- printf "%s-headless" (include "memoryfs.fullname" .) }}
{{- end }}

{{- define "memoryfs.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "memoryfs.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "memoryfs.chunkBackend" -}}
{{- $backend := .Values.node.chunkBackend -}}
{{- if $backend -}}
{{- $backend -}}
{{- else if .Values.node.diskSync.enabled -}}
buffered
{{- else -}}
memory
{{- end -}}
{{- end }}

{{- define "memoryfs.flushInterval" -}}
{{- if .Values.node.diskSync.enabled -}}
{{- .Values.node.diskSync.interval -}}
{{- else -}}
0
{{- end -}}
{{- end }}

{{- define "memoryfs.bootstrapJoinScript" -}}
ORD="${HOSTNAME##*-}"
export MEMORYFS_ID="${HOSTNAME}"
export MEMORYFS_HEADLESS_SERVICE="{{ include "memoryfs.headless" . }}"
if [ "$ORD" = "0" ]; then
  export MEMORYFS_BOOTSTRAP=true
else
  export MEMORYFS_JOIN="http://{{ include "memoryfs.fullname" . }}-0.{{ include "memoryfs.headless" . }}:{{ .Values.service.httpPort }}"
fi
exec /app/entrypoint.sh node-env
{{- end }}

{{- define "memoryfs.nodeURLs" -}}
{{- $fullname := include "memoryfs.fullname" . -}}
{{- $headless := include "memoryfs.headless" . -}}
{{- $port := .Values.service.httpPort -}}
{{- $count := int .Values.replicaCount -}}
{{- range $i, $e := until $count -}}
{{- if $i }},{{ end -}}
http://{{ $fullname }}-{{ $i }}.{{ $headless }}:{{ $port }}
{{- end -}}
{{- end }}
