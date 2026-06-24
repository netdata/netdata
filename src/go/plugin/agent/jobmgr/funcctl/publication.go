// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

type publishedFunctionKind uint8

const (
	publishedFunctionModuleMethod publishedFunctionKind = iota + 1
	publishedFunctionJobMethod
)

type publishedFunctionOwner struct {
	kind       publishedFunctionKind
	moduleName string
	jobName    string
	methodID   string
}

func moduleMethodOwner(moduleName, methodID string) publishedFunctionOwner {
	return publishedFunctionOwner{
		kind:       publishedFunctionModuleMethod,
		moduleName: moduleName,
		methodID:   methodID,
	}
}

func jobMethodOwner(moduleName, jobName, methodID string) publishedFunctionOwner {
	return publishedFunctionOwner{
		kind:       publishedFunctionJobMethod,
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
}

func newPublishedFunctionStore() *publishedFunctionStore {
	return &publishedFunctionStore{
		byName: make(map[string]publishedFunctionRecord),
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
	return record, true
}

func (s *publishedFunctionStore) generationMatches(name string, generation uint64) bool {
	record, ok := s.byName[name]
	return ok && record.generation == generation
}

func (s *publishedFunctionStore) removeOwner(owner publishedFunctionOwner) []string {
	var names []string
	for name, record := range s.byName {
		if record.owner == owner {
			names = append(names, name)
			delete(s.byName, name)
		}
	}
	return names
}

func (s *publishedFunctionStore) removeKind(kind publishedFunctionKind) []string {
	var names []string
	for name, record := range s.byName {
		if record.owner.kind == kind {
			names = append(names, name)
			delete(s.byName, name)
		}
	}
	return names
}
