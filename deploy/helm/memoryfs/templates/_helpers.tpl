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

{{- define "memoryfs.spreadAffinity" -}}
podAntiAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector:
        matchLabels:
          memoryfs.io/role: node
      topologyKey: kubernetes.io/hostname
{{- end }}

{{- define "memoryfs.podMemory" -}}
{{- printf "%dGi" (add (int .Values.node.storageGB) 1) -}}
{{- end }}

{{- define "memoryfs.diskQuotaGB" -}}
{{- if .Values.node.diskSync.enabled -}}
{{- .Values.node.storageGB -}}
{{- else -}}
0
{{- end -}}
{{- end }}

{{- define "memoryfs.nodeSecurityContext" -}}
capabilities:
  add:
    - IPC_LOCK
{{- end }}

{{- define "memoryfs.rdmaVolumes" -}}
- name: infiniband-dev
  hostPath:
    path: /dev/infiniband
    type: DirectoryOrCreate
{{- end }}

{{- define "memoryfs.rdmaVolumeMounts" -}}
- name: infiniband-dev
  mountPath: /dev/infiniband
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

{{- define "memoryfs.instanceSecretName" -}}
{{- printf "%s-instance" (include "memoryfs.fullname" .) -}}
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
export MEMORYFS_HEADLESS_SERVICE="{{ include "memoryfs.headless" . }}"
exec /app/entrypoint.sh node-env
{{- end }}

{{- define "memoryfs.nodeURLs" -}}
{{- $fullname := include "memoryfs.fullname" . -}}
{{- $headless := include "memoryfs.headless" . -}}
{{- $ns := .Release.Namespace -}}
{{- $port := .Values.service.httpPort -}}
{{- $prefix := .Values.dashboard.uriPrefix | default "" -}}
{{- $count := int .Values.replicaCount -}}
{{- range $i, $e := until $count -}}
{{- if $i }},{{ end -}}
http://{{ $fullname }}-{{ $i }}.{{ $headless }}.{{ $ns }}.svc:{{ $port }}{{ $prefix }}
{{- end -}}
{{- end }}
