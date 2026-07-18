package observation

import "github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"

type EventKind = protocol.EventKind

const (
	EventCutReached       = protocol.EventCutReached
	EventHandlerEntered   = protocol.EventHandlerEntered
	EventHandlerReturned  = protocol.EventHandlerReturned
	EventDeadlineObserved = protocol.EventDeadlineObserved
	EventWriteAttempt     = protocol.EventWriteAttempt
	EventProcessSentinel  = protocol.EventProcessSentinel
)

type PassiveEvent struct {
	Message             protocol.Event
	ObservedMonotonicNS int64
}

func NewPassiveEvent(message protocol.Event, observedMonotonicNS int64) (PassiveEvent, error) {
	if err := message.Validate(); err != nil {
		return PassiveEvent{}, err
	}
	message.Payload = append([]byte{}, message.Payload...)
	return PassiveEvent{Message: message, ObservedMonotonicNS: observedMonotonicNS}, nil
}
