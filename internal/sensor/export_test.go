package sensor

import "github.com/google/uuid"

func ExportNodeTypeID() uuid.UUID    { return gitSensorNodeTypeID }
func ExportNodeID(s *Server) uuid.UUID { return s.nodeID }
