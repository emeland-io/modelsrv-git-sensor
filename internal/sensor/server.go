package sensor

import (
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/endpoint"
	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/model"
	"go.emeland.io/modelsrv/pkg/model/node"
)

// gitSensorNodeTypeID is the stable identity for the "git-sensor" NodeType.
// All instances of this sensor share this UUID. It is intentionally hardcoded
// and must not be changed after the first deployment.
var gitSensorNodeTypeID = uuid.MustParse("bd044ec4-5e2f-5b91-a444-cc120374d21a")

// Server runs a modelsrv web endpoint backed by a local model and forwards changes to subscribers.
type Server struct {
	events *eventManager
	model  model.Model
	nodeID uuid.UUID
	log    *zap.SugaredLogger
}

// New starts a sensor server bound to listenAddr, pre-registering any provided subscriber URLs.
func New(listenAddr string, subscribers []string, log *zap.SugaredLogger) (*Server, error) {
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	em := newEventManager(log)

	sink, err := em.GetSink()
	if err != nil {
		return nil, err
	}

	m, err := model.NewModel(sink)
	if err != nil {
		return nil, err
	}

	nt := node.NewNodeType(sink, gitSensorNodeTypeID)
	nt.SetDisplayName("git-sensor")
	if err := m.AddNodeType(nt); err != nil {
		return nil, fmt.Errorf("register node type: %w", err)
	}

	nodeID := uuid.New()
	n := node.NewNode(sink, nodeID)
	n.SetDisplayName("git-sensor")
	n.SetNodeTypeByRef(nt)
	if err := m.AddNode(n); err != nil {
		return nil, fmt.Errorf("register node: %w", err)
	}

	// Register downstream subscribers only after NodeType + Node exist in the master
	// list so AddSubscriber's replay delivers them reliably (ordered, synchronous Notify).
	for _, s := range subscribers {
		if err := em.AddSubscriber(s); err != nil {
			return nil, err
		}
	}

	if err := endpoint.StartWebListener(m, em, listenAddr); err != nil {
		return nil, err
	}

	log.Infow("sensor registered",
		"nodeTypeId", gitSensorNodeTypeID,
		"nodeId", nodeID,
	)
	log.Infow("subscriber management endpoints available",
		"register", fmt.Sprintf("http://%s/api/events/register", listenAddr),
		"unregister", fmt.Sprintf("http://%s/api/events/unregister", listenAddr),
		"subscribers", fmt.Sprintf("http://%s/api/events/subscribers", listenAddr),
	)

	return &Server{events: em, model: m, nodeID: nodeID, log: log}, nil
}

// Close shuts down the sensor's HTTP listener and deregisters its Node from the model.
func (s *Server) Close() error {
	if err := s.model.DeleteNodeById(s.nodeID); err != nil {
		s.log.Warnw("failed to delete node on shutdown", "nodeId", s.nodeID, "error", err)
	}
	endpoint.StopWebListener()
	return nil
}

// Emit applies the event to this process's model (landscape API) and forwards it through the
// model sink to the event manager (master recording + subscriber notify). Apply uses the same
// Add*/Delete* paths as replication, so local state and downstream pushes stay aligned.
func (s *Server) Emit(ev events.Event) error {
	return s.model.Apply(ev)
}

// MasterEvents returns a snapshot of all events recorded in the master list.
func (s *Server) MasterEvents() []events.Event {
	if s == nil || s.events == nil {
		return nil
	}
	return s.events.snapshotMasterEvents()
}

// EventManager returns the server's event manager.
func (s *Server) EventManager() events.EventManager {
	if s == nil {
		return nil
	}
	return s.events
}
