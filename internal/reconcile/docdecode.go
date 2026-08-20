package reconcile

import (
	"regexp"

	"go.emeland.io/modelsrv/pkg/ingress"
)

// YAML files may use Event WireKind casing (e.g. ApiInstance); ingress's
// DocumentKind unmarshaler only accepts ParseResourceType names (APIInstance).
var yamlKindApiInstance = regexp.MustCompile(`(?m)^([ \t]*)kind:[ \t]+ApiInstance([ \t]*)$`)

func normalizeYAMLKindsForFileSensor(b []byte) []byte {
	return yamlKindApiInstance.ReplaceAll(b, []byte("${1}kind: APIInstance${2}"))
}

func decodeDocuments(b []byte) ([]ingress.Document, error) {
	b = normalizeYAMLKindsForFileSensor(b)
	return ingress.DecodeDocuments(b)
}
