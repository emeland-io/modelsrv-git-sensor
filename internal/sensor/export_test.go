package sensor

import (
	"github.com/google/uuid"
	"go.emeland.io/modelsrv/pkg/events"
)

func ExportNodeTypeID() uuid.UUID      { return gitSensorNodeTypeID }
func ExportNodeID(s *Server) uuid.UUID { return s.nodeID }

// Replication wire helpers (test-only exports).
var (
	ExportCloneEventForSubscriberNotify  = cloneEventForSubscriberNotify
	ExportPatchWirePayloadForReplication = func(ev *events.Event) { patchWirePayloadForReplication(nil, ev) }
	ExportSetWireInstanceID              = setWireInstanceID
	ExportNormalizeWireUUIDSliceFields   = normalizeWireUUIDSliceFields
	ExportMapValueFold                   = mapValueFold
	ExportDeleteKeyFold                  = deleteKeyFold
	ExportUUIDSliceFromWireRefs          = uuidSliceFromWireRefs
	ExportExtractUUIDScalar              = extractUUIDScalar
	ExportScalarNonEmpty                 = scalarNonEmpty
)
