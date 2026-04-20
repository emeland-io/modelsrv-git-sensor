package reconcile

import (
	"github.com/google/uuid"
	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/filesensor"
)

// Export* symbols are test hooks (this file is not compiled into non-test builds).
// Names avoid the "Test*" prefix so the Go test harness does not treat them as tests.

func ExportExtractID(rt events.ResourceType, spec map[string]any) (uuid.UUID, error) {
	return extractID(rt, spec)
}

func ExportNormalizeYAMLKindsForFileSensor(b []byte) []byte {
	return normalizeYAMLKindsForFileSensor(b)
}

func ExportDecodeDocuments(b []byte) ([]filesensor.Document, error) {
	return decodeDocuments(b)
}
