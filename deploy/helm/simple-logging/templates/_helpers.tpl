{{/*
Expand the name of the chart.
*/}}
{{- define "simple-logging.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Name of the PVC mounted for log storage.
*/}}
{{- define "simple-logging.persistenceClaimName" -}}
{{- if .Values.persistence.existingClaim }}
{{- .Values.persistence.existingClaim }}
{{- else }}
{{- printf "%s-%s" (include "simple-logging.fullname" .) .Values.persistence.claimSuffix | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create a default fully qualified app name, truncated to 63 chars per DNS spec.
If the release name already contains the chart name it is used as-is.
*/}}
{{- define "simple-logging.fullname" -}}
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
Create chart label (name + version).
*/}}
{{- define "simple-logging.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "simple-logging.labels" -}}
helm.sh/chart: {{ include "simple-logging.chart" . }}
{{ include "simple-logging.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels — used by the Deployment selector and the Service.
*/}}
{{- define "simple-logging.selectorLabels" -}}
app.kubernetes.io/name: {{ include "simple-logging.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the ServiceAccount to use.
*/}}
{{- define "simple-logging.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "simple-logging.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Fail with an upgrade hint when a values key removed in 0.14.0 (the single-
binary release) is still set,
rather than silently ignoring it.
*/}}
{{- define "simple-logging.failOnRemovedValues" -}}
{{- $removed := list }}
{{- if .Values.grpcWebUrl }}
{{- $removed = append $removed "grpcWebUrl (the UI now always calls the API on its own origin)" }}
{{- end }}
{{- if (.Values.ingress).grpcPathPrefix }}
{{- $removed = append $removed "ingress.grpcPathPrefix (the ingress now has a single / path)" }}
{{- end }}
{{- if (.Values.config).grpcWebPort }}
{{- $removed = append $removed "config.grpcWebPort (use config.port)" }}
{{- end }}
{{- if (.Values.config).restDebug }}
{{- $removed = append $removed "config.restDebug (every RPC can now be called as JSON with curl; see the README)" }}
{{- end }}
{{- if or (.Values.service).httpPort (.Values.service).grpcWebPort }}
{{- $removed = append $removed "service.httpPort / service.grpcWebPort (use service.port)" }}
{{- end }}
{{- if $removed }}
{{- fail (printf "\n\nThese values were removed in chart 0.14.0, when the image became a single binary serving the UI and API on one port. Remove them from your values:\n  - %s\n" (join "\n  - " $removed)) }}
{{- end }}
{{- end }}
