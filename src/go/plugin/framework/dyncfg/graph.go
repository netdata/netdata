// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

const MaximumGraphMutationChanges = 256

var (
	ErrGraphMutationActive   = errors.New("dyncfg graph: mutation active")
	ErrGraphMutationConsumed = errors.New("dyncfg graph: mutation consumed")
	ErrGraphNoChange         = errors.New("dyncfg graph: no change")
)

type GraphConfig struct {
	ID      string
	Module  string
	Name    string
	Status  string
	Payload []byte
}

type GraphRecord struct {
	ID      string
	Module  string
	Name    string
	Status  string
	payload string
}

func (record GraphRecord) Payload() string {
	return record.payload
}

type exposedGraphKey struct {
	module string
	name   string
}

func (record GraphRecord) exposedKey() exposedGraphKey {
	return exposedGraphKey{module: record.Module, name: record.Name}
}

// Graph owns the one atomic ID/exposed-index DynCfg postimage.
type Graph struct {
	mu          sync.RWMutex
	records     map[string]GraphRecord
	exposed     map[exposedGraphKey]string
	version     uint64
	nextToken   uint64
	activeToken uint64
}

type GraphChange struct {
	ID     string
	Config *GraphConfig
}

type GraphMutation struct {
	graph       *Graph
	token       uint64
	baseVersion uint64
	changes     []preparedGraphChange
}

type preparedGraphChange struct {
	id     string
	remove bool
	record GraphRecord
}

func NewGraph(configs []GraphConfig) (*Graph, error) {
	graph := &Graph{
		records: make(map[string]GraphRecord, len(configs)),
		exposed: make(map[exposedGraphKey]string, len(configs)),
		version: 1,
	}
	for _, config := range configs {
		record, err := prepareGraphRecord(config)
		if err != nil {
			return nil, err
		}
		if _, ok := graph.records[record.ID]; ok {
			return nil, errors.New("dyncfg graph: duplicate config")
		}
		key := record.exposedKey()
		if _, ok := graph.exposed[key]; ok {
			return nil, errors.New("dyncfg graph: duplicate exposed config")
		}
		graph.records[record.ID] = record
		graph.exposed[key] = record.ID
	}
	return graph, nil
}

func (graph *Graph) Lookup(id string) (GraphRecord, bool) {
	graph.mu.RLock()
	record, ok := graph.records[id]
	graph.mu.RUnlock()
	return record, ok
}

func (graph *Graph) IDs() []string {
	graph.mu.RLock()
	ids := make([]string, 0, len(graph.records))
	for id := range graph.records {
		ids = append(ids, id)
	}
	graph.mu.RUnlock()
	sort.Strings(ids)
	return ids
}

func (graph *Graph) PrepareMutation(changes []GraphChange) (GraphMutation, error) {
	if len(changes) == 0 || len(changes) > MaximumGraphMutationChanges {
		return GraphMutation{}, errors.New("dyncfg graph: invalid mutation size")
	}
	graph.mu.Lock()
	defer graph.mu.Unlock()
	if graph.activeToken != 0 {
		return GraphMutation{}, ErrGraphMutationActive
	}

	prepared := make([]preparedGraphChange, len(changes))
	byID := make(map[string]preparedGraphChange, len(changes))
	for index, change := range changes {
		id := strings.TrimSpace(change.ID)
		if id == "" {
			return GraphMutation{}, errors.New("dyncfg graph: empty change ID")
		}
		if _, ok := byID[id]; ok {
			return GraphMutation{}, errors.New("dyncfg graph: duplicate change ID")
		}
		item := preparedGraphChange{id: id, remove: change.Config == nil}
		if change.Config != nil {
			if change.Config.ID != id {
				return GraphMutation{}, errors.New("dyncfg graph: change/config ID mismatch")
			}
			record, err := prepareGraphRecord(*change.Config)
			if err != nil {
				return GraphMutation{}, err
			}
			item.record = record
		}
		prepared[index] = item
		byID[id] = item
	}

	desiredExposed := make(map[exposedGraphKey]string, len(changes))
	changed := false
	for _, change := range prepared {
		current, exists := graph.records[change.id]
		if change.remove {
			changed = changed || exists
			continue
		}
		changed = changed || !exists || !graphRecordsEqual(current, change.record)
		key := change.record.exposedKey()
		if other := desiredExposed[key]; other != "" && other != change.id {
			return GraphMutation{}, errors.New("dyncfg graph: duplicate postimage exposed config")
		}
		desiredExposed[key] = change.id
		if occupant := graph.exposed[key]; occupant != "" && occupant != change.id {
			occupantChange, moving := byID[occupant]
			if !moving || (!occupantChange.remove && occupantChange.record.exposedKey() == key) {
				return GraphMutation{}, errors.New("dyncfg graph: exposed config conflict")
			}
		}
	}
	if !changed {
		return GraphMutation{}, ErrGraphNoChange
	}
	graph.nextToken++
	if graph.nextToken == 0 {
		return GraphMutation{}, errors.New("dyncfg graph: mutation token wrapped")
	}
	graph.activeToken = graph.nextToken
	return GraphMutation{
		graph:       graph,
		token:       graph.nextToken,
		baseVersion: graph.version,
		changes:     prepared,
	}, nil
}

func (graph *Graph) Commit(mutation GraphMutation) error {
	graph.mu.Lock()
	defer graph.mu.Unlock()
	if mutation.graph != graph ||
		mutation.token == 0 ||
		graph.activeToken != mutation.token ||
		graph.version != mutation.baseVersion {
		return ErrGraphMutationConsumed
	}
	for _, change := range mutation.changes {
		if current, ok := graph.records[change.id]; ok {
			delete(graph.exposed, current.exposedKey())
		}
	}
	for _, change := range mutation.changes {
		if change.remove {
			delete(graph.records, change.id)
		} else {
			graph.records[change.id] = change.record
		}
	}
	for _, change := range mutation.changes {
		if !change.remove {
			graph.exposed[change.record.exposedKey()] = change.id
		}
	}
	graph.version++
	graph.activeToken = 0
	return nil
}

func (graph *Graph) Abort(mutation GraphMutation) error {
	graph.mu.Lock()
	defer graph.mu.Unlock()
	if mutation.graph != graph ||
		mutation.token == 0 ||
		graph.activeToken != mutation.token ||
		graph.version != mutation.baseVersion {
		return ErrGraphMutationConsumed
	}
	graph.activeToken = 0
	return nil
}

func prepareGraphRecord(config GraphConfig) (GraphRecord, error) {
	if strings.TrimSpace(config.ID) == "" ||
		strings.TrimSpace(config.Module) == "" ||
		strings.TrimSpace(config.Name) == "" {
		return GraphRecord{}, errors.New("dyncfg graph: invalid config")
	}
	return GraphRecord{
		ID: config.ID, Module: config.Module, Name: config.Name,
		Status: config.Status, payload: string(config.Payload),
	}, nil
}

func graphRecordsEqual(left, right GraphRecord) bool {
	return left.ID == right.ID &&
		left.Module == right.Module &&
		left.Name == right.Name &&
		left.Status == right.Status &&
		left.payload == right.payload
}
