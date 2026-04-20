package reconcile

import (
	"regexp"

	"go.emeland.io/modelsrv/pkg/filesensor"
)

// YAML files may use Event WireKind casing (e.g. ApiInstance); filesensor's
// DocumentKind unmarshaler only accepts ParseResourceType names (APIInstance).
var yamlKindApiInstance = regexp.MustCompile(`(?m)^([ \t]*)kind:[ \t]+ApiInstance([ \t]*)$`)

func normalizeYAMLKindsForFileSensor(b []byte) []byte {
	return yamlKindApiInstance.ReplaceAll(b, []byte("${1}kind: APIInstance${2}"))
}

func decodeDocuments(b []byte) ([]filesensor.Document, error) {
	b = normalizeYAMLKindsForFileSensor(b)
	return filesensor.DecodeDocuments(b)
}
