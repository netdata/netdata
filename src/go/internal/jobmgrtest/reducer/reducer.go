package reducer

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
)

type CompletedOperation struct {
	Offer  observation.OfferedRequest
	Result observation.FunctionResult
	Events []observation.PassiveEvent
}

type operationState struct {
	offer     observation.OfferedRequest
	result    observation.FunctionResult
	events    []observation.PassiveEvent
	offered   bool
	resultSet bool
}

type Reducer struct {
	limit     int
	offered   int
	completed int
	states    []operationState
	byUID     map[string]int
	prepared  bool
	eventGen  uint64
	eventSeq  uint64
}

func New(limit int) (*Reducer, error) {
	if limit <= 0 {
		return nil, errors.New("evaluator reducer: limit must be positive")
	}
	return &Reducer{
		limit:  limit,
		states: make([]operationState, limit),
		byUID:  make(map[string]int, limit),
	}, nil
}

func NewPrepared(schedule *contract.PerformanceSchedule) (*Reducer, error) {
	if schedule == nil {
		return nil, errors.New("evaluator reducer: nil schedule")
	}
	reduced, err := New(len(schedule.Operations))
	if err != nil {
		return nil, err
	}
	reduced.prepared = true
	for sequence, operation := range schedule.Operations {
		if operation.Sequence != sequence || operation.UID == "" {
			return nil, fmt.Errorf("evaluator reducer: invalid prepared operation %d", sequence)
		}
		if _, exists := reduced.byUID[operation.UID]; exists {
			return nil, fmt.Errorf("evaluator reducer: duplicate prepared UID %s", operation.UID)
		}
		reduced.byUID[operation.UID] = sequence
		reduced.states[sequence].offer = observation.OfferedRequest{
			Sequence:         sequence,
			Class:            string([]byte{byte(operation.Class)}),
			Key:              operation.Key,
			UID:              operation.UID,
			RequestSHA256:    operation.RequestSHA256,
			FollowupSHA256:   operation.FollowupSHA256,
			UsefulWorkSHA256: operation.UsefulWorkSHA256,
		}
		switch operation.Class {
		case contract.ClassCancel, contract.ClassDeadline:
			reduced.states[sequence].events = make([]observation.PassiveEvent, 0, 1)
		}
	}
	return reduced, nil
}

func (reducer *Reducer) Offer(offer observation.OfferedRequest) error {
	if offer.Sequence < 0 || offer.UID == "" || offer.Class == "" || offer.Key == "" || offer.OfferedMonoNS < 0 {
		return errors.New("evaluator reducer: incomplete offered request")
	}
	if offer.Sequence >= reducer.limit {
		return fmt.Errorf("evaluator reducer: sequence %d outside operation limit", offer.Sequence)
	}
	state := &reducer.states[offer.Sequence]
	if state.offered {
		return fmt.Errorf("evaluator reducer: duplicate sequence %d", offer.Sequence)
	}
	if reducer.prepared {
		expected := state.offer
		if offer.Sequence != expected.Sequence ||
			offer.Class != expected.Class ||
			offer.Key != expected.Key ||
			offer.UID != expected.UID ||
			offer.RequestSHA256 != expected.RequestSHA256 ||
			offer.FollowupSHA256 != expected.FollowupSHA256 ||
			offer.UsefulWorkSHA256 != expected.UsefulWorkSHA256 {
			return fmt.Errorf("evaluator reducer: offer differs from prepared sequence %d", offer.Sequence)
		}
		state.offer.OfferedMonoNS = offer.OfferedMonoNS
	} else {
		if _, exists := reducer.byUID[offer.UID]; exists {
			return fmt.Errorf("evaluator reducer: duplicate UID %s", offer.UID)
		}
		reducer.byUID[offer.UID] = offer.Sequence
		state.offer = offer
	}
	state.offered = true
	reducer.offered++
	return nil
}

func (reducer *Reducer) ObserveResult(result observation.FunctionResult) error {
	sequence, exists := reducer.byUID[result.UID]
	if !exists {
		return fmt.Errorf("evaluator reducer: result for unknown UID %s", result.UID)
	}
	state := &reducer.states[sequence]
	if !state.offered {
		return fmt.Errorf("evaluator reducer: result for unoffered UID %s", result.UID)
	}
	if state.resultSet {
		return fmt.Errorf("evaluator reducer: duplicate result for UID %s", result.UID)
	}
	if result.ReadReturnMonoNS < 0 {
		return fmt.Errorf("evaluator reducer: result has a negative timestamp for UID %s", result.UID)
	}
	state.result = cloneResult(result)
	state.resultSet = true
	reducer.completed++
	return nil
}

func (reducer *Reducer) SetFollowup(uid string, observedMonotonicNS int64) error {
	sequence, exists := reducer.byUID[uid]
	if !exists {
		return fmt.Errorf("evaluator reducer: follow-up for unknown UID %s", uid)
	}
	state := &reducer.states[sequence]
	if !state.offered {
		return fmt.Errorf("evaluator reducer: follow-up for unoffered UID %s", uid)
	}
	if state.offer.FollowupMonoNS != 0 {
		return fmt.Errorf("evaluator reducer: duplicate follow-up for UID %s", uid)
	}
	if observedMonotonicNS < state.offer.OfferedMonoNS {
		return fmt.Errorf("evaluator reducer: follow-up precedes offer for UID %s", uid)
	}
	state.offer.FollowupMonoNS = observedMonotonicNS
	return nil
}

func (reducer *Reducer) ObserveEvent(event observation.PassiveEvent) error {
	if event.Message.Token == "" {
		return errors.New("evaluator reducer: operation event lacks public token")
	}
	sequence, exists := reducer.byUID[event.Message.Token]
	if !exists {
		return fmt.Errorf("evaluator reducer: event for unknown token %s", event.Message.Token)
	}
	state := &reducer.states[sequence]
	if !state.offered {
		return fmt.Errorf("evaluator reducer: event for unoffered UID %s", event.Message.Token)
	}
	if event.ObservedMonotonicNS < 0 {
		return fmt.Errorf("evaluator reducer: event has a negative timestamp for UID %s", event.Message.Token)
	}
	if reducer.eventGen == 0 {
		reducer.eventGen = event.Message.ProcessGeneration
	} else if event.Message.ProcessGeneration != reducer.eventGen {
		return fmt.Errorf("evaluator reducer: event generation changed from %d to %d", reducer.eventGen, event.Message.ProcessGeneration)
	}
	if event.Message.EventSeq <= reducer.eventSeq {
		return fmt.Errorf("evaluator reducer: nonmonotonic global event sequence %d after %d", event.Message.EventSeq, reducer.eventSeq)
	}
	if len(state.events) > 0 && event.Message.EventSeq <= state.events[len(state.events)-1].Message.EventSeq {
		return fmt.Errorf("evaluator reducer: nonmonotonic event sequence for UID %s", event.Message.Token)
	}
	reducer.eventSeq = event.Message.EventSeq
	state.events = append(state.events, cloneEvent(event))
	return nil
}

func (reducer *Reducer) HasEvent(uid string, kind observation.EventKind) bool {
	sequence, exists := reducer.byUID[uid]
	if !exists {
		return false
	}
	state := &reducer.states[sequence]
	for _, event := range state.events {
		if event.Message.Kind == kind {
			return true
		}
	}
	return false
}

func (reducer *Reducer) Completed() []CompletedOperation {
	completed := make([]CompletedOperation, 0, reducer.completed)
	for sequence := range reducer.states {
		state := &reducer.states[sequence]
		if !state.resultSet {
			continue
		}
		events := make([]observation.PassiveEvent, len(state.events))
		for index, event := range state.events {
			events[index] = cloneEvent(event)
		}
		completed = append(completed, CompletedOperation{
			Offer: state.offer, Result: cloneResult(state.result), Events: events,
		})
	}
	return completed
}

func (reducer *Reducer) Outstanding() int {
	return reducer.offered - reducer.completed
}

func cloneResult(result observation.FunctionResult) observation.FunctionResult {
	result.Payload = append([]byte(nil), result.Payload...)
	result.Deferred = append([]byte(nil), result.Deferred...)
	result.Raw = append([]byte(nil), result.Raw...)
	return result
}

func cloneEvent(event observation.PassiveEvent) observation.PassiveEvent {
	event.Message.Payload = append([]byte(nil), event.Message.Payload...)
	return event
}
