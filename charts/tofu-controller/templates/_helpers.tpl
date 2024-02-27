{{/*
Expand the name of the chart.
*/}}
{{- define "tofu-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "tofu-controller.fullname" -}}
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
{{- define "tofu-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "tofu-controller.labels" -}}
helm.sh/chart: {{ include "tofu-controller.chart" . }}
{{ include "tofu-controller.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
control-plane: controller
{{- end }}

{{/*
Selector labels
*/}}
{{- define "tofu-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tofu-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the controller service account to use
*/}}
{{- define "tofu-controller.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tofu-controller.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the runner service account to use
*/}}
{{- define "tofu-controller.runner.serviceAccountName" -}}
{{- if .Values.runner.serviceAccount.create }}
{{- default "tf-runner" .Values.runner.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.runner.serviceAccount.name }}
{{- end }}
{{- end }}


{{/*
Create a unique list of runner allowed namespaces
*/}}
{{- define "tofu-controller.runner.allowedNamespaces" -}}
{{- $allowedNamespaces := append .Values.runner.serviceAccount.allowedNamespaces .Release.Namespace -}}
{{- $allowedNamespaces := append $allowedNamespaces "flux-system" -}}
{{- $allowedNamespaces = $allowedNamespaces | uniq -}}
{{ toJson $allowedNamespaces }}
{{- end }}

{{/*
Expand the name of the chart.
*/}}
{{- define "planner.name" -}}
{{ include "tofu-controller.name" . }}-branch-planner
{{- end }}

{{/*
Create a default fully qualified planner name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "planner.fullname" -}}
{{ include "tofu-controller.fullname" . }}-branch-planner
{{- end }}

{{/*
Common labels
*/}}
{{- define "planner.labels" -}}
helm.sh/chart: {{ include "tofu-controller.chart" . }}
{{ include "planner.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "planner.selectorLabels" -}}
app.kubernetes.io/name: {{ include "planner.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Inject pod namespace
*/}}
{{- define "pod-namespace" }}
- name: RUNTIME_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
{{- end }}
