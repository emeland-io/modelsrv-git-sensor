package reconcile

import "go.emeland.io/modelsrv/pkg/events"

// Emitter is implemented by any value that can receive a domain event and
// forward it to downstream subscribers. *sensor.Server satisfies this interface.
type Emitter interface {
	Emit(events.Event) error
}
