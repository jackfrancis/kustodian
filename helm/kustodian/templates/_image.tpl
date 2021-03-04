{{- define "image.full" -}}
{{- printf "%s/%s:%s" .imageRegistry .imageRepository .imageTag -}}
{{- end -}}
