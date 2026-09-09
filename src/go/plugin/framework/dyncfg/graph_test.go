// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"testing"
)

func lookupExposedGraph(graph *Graph, module, name string) (GraphRecord, bool) {
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	id := graph.exposed[exposedGraphKey{module: module, name: name}]
	record, ok := graph.records[id]
	return record, ok
}

func graphVersion(graph *Graph) uint64 {
	graph.mu.RLock()
	defer graph.mu.RUnlock()
	return graph.version
}

func TestDynCfgAtomicIndexes(t *testing.T) {
	graph, err := NewGraph([]GraphConfig{{
		ID: "old", Module: "m", Name: "job", Payload: []byte("old"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := graph.PrepareMutation([]GraphChange{
		{ID: "old"},
		{ID: "new", Config: &GraphConfig{
			ID: "new", Module: "m", Name: "job", Payload: []byte("new"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := graph.Lookup("new"); ok {
		t.Fatal("private postimage visible before commit")
	}
	if record, ok := lookupExposedGraph(graph, "m", "job"); !ok || record.ID != "old" {
		t.Fatalf("pre-commit exposed record=%#v ok=%v", record, ok)
	}
	if err := graph.Commit(mutation); err != nil {
		t.Fatal(err)
	}
	if _, ok := graph.Lookup("old"); ok {
		t.Fatal("removed ID survived commit")
	}
	if record, ok := lookupExposedGraph(graph, "m", "job"); !ok ||
		record.ID != "new" ||
		record.Payload() != "new" {
		t.Fatalf("post-commit exposed record=%#v ok=%v", record, ok)
	}
}

func TestDynCfgGraphMutationFailuresPreservePriorGraph(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T, *Graph)
	}{
		"conflict": {
			run: func(t *testing.T, graph *Graph) {
				if _, err := graph.PrepareMutation([]GraphChange{{
					ID: "b", Config: &GraphConfig{ID: "b", Module: "m", Name: "a"},
				}}); err == nil {
					t.Fatal("conflicting exposed key unexpectedly prepared")
				}
			},
		},
		"abort": {
			run: func(t *testing.T, graph *Graph) {
				mutation, err := graph.PrepareMutation([]GraphChange{{ID: "a"}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := graph.PrepareMutation([]GraphChange{{ID: "b"}}); !errors.Is(err, ErrGraphMutationActive) {
					t.Fatalf("second mutation error=%v, want ErrGraphMutationActive", err)
				}
				if err := graph.Abort(mutation); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			graph, err := NewGraph([]GraphConfig{
				{ID: "a", Module: "m", Name: "a"},
				{ID: "b", Module: "m", Name: "b"},
			})
			if err != nil {
				t.Fatal(err)
			}
			test.run(t, graph)
			if record, ok := lookupExposedGraph(graph, "m", "a"); !ok || record.ID != "a" {
				t.Fatalf("graph changed after failed mutation: %#v ok=%v", record, ok)
			}
		})
	}
}

func TestDynCfgGraphNoChangeIsTyped(t *testing.T) {
	config := GraphConfig{
		ID: "a", Module: "m", Name: "a", Status: "running", Payload: []byte("payload"),
	}
	graph, err := NewGraph([]GraphConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.PrepareMutation([]GraphChange{{ID: "a", Config: &config}}); !errors.Is(err, ErrGraphNoChange) {
		t.Fatalf("no-change error=%v, want ErrGraphNoChange", err)
	}
	if version := graphVersion(graph); version != 1 {
		t.Fatalf("version=%d after no-change", version)
	}
}

func TestDynCfgGraphOwnsPayloadAndLookupAllocatesNothing(t *testing.T) {
	payload := []byte("payload")
	graph, err := NewGraph([]GraphConfig{{
		ID: "a", Module: "m", Name: "a", Payload: payload,
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	record, ok := graph.Lookup("a")
	if !ok || record.Payload() != "payload" {
		t.Fatalf("record=%#v ok=%v", record, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = graph.Lookup("a")
	}); allocations != 0 {
		t.Fatalf("lookup allocations=%f, want 0", allocations)
	}
}

func BenchmarkBDynCfgGraph(b *testing.B) {
	graph, err := NewGraph([]GraphConfig{{
		ID: "a", Module: "m", Name: "a", Payload: []byte("payload"),
	}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = graph.Lookup("a")
	}
}
