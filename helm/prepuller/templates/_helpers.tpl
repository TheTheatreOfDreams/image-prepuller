{{- define "prepuller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "prepuller.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "prepuller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "prepuller.labels" -}}
helm.sh/chart: {{ include "prepuller.chart" . }}
app.kubernetes.io/name: {{ include "prepuller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "prepuller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "prepuller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "prepuller.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{- define "prepuller.controllerServiceAccountName" -}}
{{- if .Values.serviceAccount.controller.create -}}
{{- default (printf "%s-controller-manager" (include "prepuller.fullname" .)) .Values.serviceAccount.controller.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.controller.name -}}
{{- end -}}
{{- end -}}

{{- define "prepuller.agentServiceAccountName" -}}
{{- if .Values.serviceAccount.agent.create -}}
{{- default (printf "%s-agent" (include "prepuller.fullname" .)) .Values.serviceAccount.agent.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.agent.name -}}
{{- end -}}
{{- end -}}
