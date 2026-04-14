package sensor

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/client"
	"go.emeland.io/modelsrv/pkg/endpoint"
	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/model"
	"go.emeland.io/modelsrv/pkg/model/node"
)

// gitSensorNodeTypeID is the stable identity for the "git-sensor" NodeType.
// All instances of this sensor share this UUID. It is intentionally hardcoded
// and must not be changed after the first deployment.
var gitSensorNodeTypeID = uuid.MustParse("bd044ec4-5e2f-5b91-a444-cc120374d21a")

// sensorServer runs a modelsrv web endpoint and forwards events to subscribers.
type sensorServer struct {
	events *eventManager
	model  model.Model
	nodeID uuid.UUID // this process's Node UUID, generated at startup
	log    *zap.SugaredLogger
}

func New(listenAddr string, subscribers []string, log *zap.SugaredLogger) (*sensorServer, error) {
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	em := newEventManager(log)
	for _, s := range subscribers {
		if err := em.AddSubscriber(s); err != nil {
			return nil, err
		}
	}

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

	if err := endpoint.StarWebListener(m, em, listenAddr); err != nil {
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

	return &sensorServer{events: em, model: m, nodeID: nodeID, log: log}, nil
}

func (s *sensorServer) Close() error {
	if err := s.model.DeleteNodeById(s.nodeID); err != nil {
		s.log.Warnw("failed to delete node on shutdown", "nodeId", s.nodeID, "error", err)
	}
	endpoint.StopWebListener()
	return nil
}

// Emit forwards an event into the sensor's event manager (which records and pushes to subscribers).
func (s *sensorServer) Emit(ev events.Event) error {
	sink, err := s.events.GetSink()
	if err != nil {
		return err
	}
	return sink.Receive(ev.ResourceType, ev.Operation, ev.ResourceId, ev.Objects...)
}

// TestOnlyEvents returns the event manager for tests.
func TestOnlyEvents(s *sensorServer) events.EventManager {
	if s == nil {
		return nil
	}
	return s.events
}

// TestOnlyNodeTypeID returns the hardcoded NodeType UUID for tests.
func TestOnlyNodeTypeID() uuid.UUID {
	return gitSensorNodeTypeID
}

// TestOnlyNodeID returns the per-process Node UUID for tests.
func TestOnlyNodeID(s *sensorServer) uuid.UUID {
	return s.nodeID
}

// TestOnlyMasterEvents returns all events recorded in the master list for tests.
func TestOnlyMasterEvents(s *sensorServer) []events.Event {
	return s.events.master.GetEvents()
}

// ---- Event manager implementation (mirrors modelsrv internal/events, but local to this repo) ----

type eventManager struct {
	log         *zap.SugaredLogger
	sequence    uint64
	subscribers []events.Subscriber
	master      *events.ListSink
	sink        events.EventSink
}

var _ events.EventManager = (*eventManager)(nil)

func newEventManager(log *zap.SugaredLogger) *eventManager {
	if log == nil {
		log = zap.NewNop().Sugar()
	}
	return &eventManager{
		log:         log,
		subscribers: make([]events.Subscriber, 0),
		master:      events.NewListSink(),
	}
}

func (e *eventManager) GetCurrentSequenceId(ctx context.Context) (uint64, error) {
	return e.sequence, nil
}

func (e *eventManager) IncrementSequenceId(ctx context.Context) error {
	e.sequence++
	return nil
}

func (e *eventManager) SetSinkFactory(factory func() (events.EventSink, error)) {
	// Not used; this manager always uses its own recording sink.
}

func (e *eventManager) GetSink() (events.EventSink, error) {
	if e.sink == nil {
		e.sink = &recordingSink{mgr: e}
	}
	return e.sink, nil
}

func (e *eventManager) GetSubscribers() []events.Subscriber {
	out := make([]events.Subscriber, len(e.subscribers))
	copy(out, e.subscribers)
	return out
}

func (e *eventManager) AddSubscriber(subURL string) error {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return fmt.Errorf("empty subscriber url")
	}
	if _, err := url.Parse(subURL); err != nil {
		return fmt.Errorf("invalid subscriber url %q: %w", subURL, err)
	}
	for _, sub := range e.subscribers {
		if sub.GetURL() == subURL {
			e.log.Infow("subscriber already registered", "subscriber", subURL)
			return nil
		}
	}
	sub, err := newSubscriber(subURL, e.log)
	if err != nil {
		return err
	}
	e.subscribers = append(e.subscribers, sub)
	e.log.Infow("subscriber registered", "subscriber", sub.GetURL())

	// Replay past events.
	past := e.master.GetEvents()
	if len(past) == 0 {
		e.log.Infow("subscriber replay complete", "subscriber", sub.GetURL(), "replayed", 0)
		return nil
	}
	var okCount, errCount int
	for _, ev := range past {
		evCopy := ev
		if err := sub.Notify(context.Background(), &evCopy); err != nil {
			errCount++
			e.log.Errorw("subscriber replay failed",
				"subscriber", sub.GetURL(),
				"kind", evCopy.ResourceType.String(),
				"operation", evCopy.Operation.WireOperation(),
				"id", evCopy.ResourceId.String(),
				"error", err,
			)
			continue
		}
		okCount++
	}
	e.log.Infow("subscriber replay complete", "subscriber", sub.GetURL(), "replayed", len(past), "ok", okCount, "failed", errCount)

	return nil
}

func (e *eventManager) RemoveSubscriber(url string) error {
	for i, sub := range e.subscribers {
		if sub.GetURL() == url {
			e.subscribers = append(e.subscribers[:i], e.subscribers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("subscriber %s not found", url)
}

type recordingSink struct {
	mgr *eventManager
}

var _ events.EventSink = (*recordingSink)(nil)

func (r *recordingSink) Receive(resType events.ResourceType, op events.Operation, resourceId uuid.UUID, objects ...any) error {
	ev := events.Event{
		ResourceType: resType,
		Operation:    op,
		ResourceId:   resourceId,
		Objects:      objects,
	}
	_ = r.mgr.master.Receive(resType, op, resourceId, objects...)
	r.mgr.sequence++

	r.mgr.log.Infow("event recorded", "kind", resType.String(), "operation", op.WireOperation(), "id", resourceId.String(), "subscribers", len(r.mgr.subscribers))

	for _, sub := range r.mgr.subscribers {
		s := sub
		evCopy := ev
		go func() {
			if err := s.Notify(context.Background(), &evCopy); err != nil {
				r.mgr.log.Errorw("subscriber notify failed",
					"subscriber", s.GetURL(),
					"kind", evCopy.ResourceType.String(),
					"operation", evCopy.Operation.WireOperation(),
					"id", evCopy.ResourceId.String(),
					"error", err,
				)
				return
			}
			r.mgr.log.Infow("subscriber notified",
				"subscriber", s.GetURL(),
				"kind", evCopy.ResourceType.String(),
				"operation", evCopy.Operation.WireOperation(),
				"id", evCopy.ResourceId.String(),
			)
		}()
	}
	return nil
}

type subscriber struct {
	url    string
	id     uuid.UUID
	status string
	c      *client.ModelSrvClient
	log    *zap.SugaredLogger
}

var _ events.Subscriber = (*subscriber)(nil)

func newSubscriber(url string, log *zap.SugaredLogger) (*subscriber, error) {
	c, err := client.NewModelSrvClient(url)
	if err != nil {
		return nil, err
	}
	return &subscriber{
		url:    url,
		id:     uuid.New(),
		status: "active",
		c:      c,
		log:    log,
	}, nil
}

func (s *subscriber) Notify(ctx context.Context, event *events.Event) error {
	return s.c.PostEvent(ctx, event)
}

func (s *subscriber) GetURL() string    { return s.url }
func (s *subscriber) GetId() uuid.UUID  { return s.id }
func (s *subscriber) GetStatus() string { return s.status }

