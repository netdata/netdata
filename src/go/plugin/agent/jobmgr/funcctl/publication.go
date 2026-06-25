// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import "sort"

type publishedFunctionKind uint8

const (
	publishedFunctionShared publishedFunctionKind = iota + 1
	publishedFunctionAgent
	publishedFunctionInstance
)

type publishedFunctionOwner struct {
	kind       publishedFunctionKind
	moduleName string
	jobName    string
	methodID   string
}

func sharedFunctionOwner(moduleName, methodID string) publishedFunctionOwner {
	return publishedFunctionOwner{
		kind:       publishedFunctionShared,
		moduleName: moduleName,
		methodID:   methodID,
	}
}

func agentFunctionOwner(moduleName, methodID string) publishedFunctionOwner {
	return publishedFunctionOwner{
		kind:       publishedFunctionAgent,
		moduleName: moduleName,
		methodID:   methodID,
	}
}

func instanceFunctionOwner(moduleName, jobName, methodID string) publishedFunctionOwner {
	return publishedFunctionOwner{
		kind:       publishedFunctionInstance,
		moduleName: moduleName,
		jobName:    jobName,
		methodID:   methodID,
	}
}

type publishedFunctionRecord struct {
	owner      publishedFunctionOwner
	generation uint64
}

type publishedFunctionStore struct {
	nextGeneration uint64
	byName         map[string]publishedFunctionRecord
	byModule       map[string]map[string]struct{}
}

func newPublishedFunctionStore() *publishedFunctionStore {
	return &publishedFunctionStore{
		byName:   make(map[string]publishedFunctionRecord),
		byModule: make(map[string]map[string]struct{}),
	}
}

func (s *publishedFunctionStore) has(name string) bool {
	_, ok := s.byName[name]
	return ok
}

func (s *publishedFunctionStore) all(names []string) bool {
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if !s.has(name) {
			return false
		}
	}
	return true
}

func (s *publishedFunctionStore) add(name string, owner publishedFunctionOwner) (publishedFunctionRecord, bool) {
	if existing, ok := s.byName[name]; ok {
		return existing, false
	}
	s.nextGeneration++
	record := publishedFunctionRecord{
		owner:      owner,
		generation: s.nextGeneration,
	}
	s.byName[name] = record
	if s.byModule[owner.moduleName] == nil {
		s.byModule[owner.moduleName] = make(map[string]struct{})
	}
	s.byModule[owner.moduleName][name] = struct{}{}
	return record, true
}

func (s *publishedFunctionStore) generationMatches(name string, generation uint64) bool {
	record, ok := s.byName[name]
	return ok && record.generation == generation
}

func (s *publishedFunctionStore) removeKinds(kinds ...publishedFunctionKind) []string {
	allowed := make(map[publishedFunctionKind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}

	var names []string
	for name, record := range s.byName {
		if _, ok := allowed[record.owner.kind]; ok {
			names = append(names, name)
			s.removeName(name)
		}
	}
	sort.Strings(names)
	return names
}

func (s *publishedFunctionStore) moduleRecords(moduleName string) map[string]publishedFunctionRecord {
	records := make(map[string]publishedFunctionRecord)
	for name := range s.byModule[moduleName] {
		records[name] = s.byName[name]
	}
	return records
}

func (s *publishedFunctionStore) removeName(name string) {
	record, ok := s.byName[name]
	if !ok {
		return
	}
	delete(s.byName, name)
	if names := s.byModule[record.owner.moduleName]; names != nil {
		delete(names, name)
		if len(names) == 0 {
			delete(s.byModule, record.owner.moduleName)
		}
	}
}
