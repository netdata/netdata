// SPDX-License-Identifier: GPL-3.0-or-later

package metrix_test

import (
	"reflect"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const metrixPkgPath = "github.com/netdata/netdata/go/plugins/pkg/metrix"

func TestPublicTypesAreRootDefined(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
	}{
		{name: "Label", typ: reflect.TypeFor[metrix.Label]()},
		{name: "LabelSet", typ: reflect.TypeFor[metrix.LabelSet]()},
		{name: "HostScope", typ: reflect.TypeFor[metrix.HostScope]()},
		{name: "SeriesID", typ: reflect.TypeFor[metrix.SeriesID]()},
		{name: "Reader", typ: reflect.TypeFor[metrix.Reader]()},
		{name: "CollectorStore", typ: reflect.TypeFor[metrix.CollectorStore]()},
		{name: "RuntimeStore", typ: reflect.TypeFor[metrix.RuntimeStore]()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.PkgPath(); got != metrixPkgPath {
				t.Fatalf("%s package path = %q, want %q", tt.name, got, metrixPkgPath)
			}
		})
	}
}

func TestCollectorStorePublicContract(t *testing.T) {
	store := metrix.NewCollectorStore(
		metrix.WithExpireAfterSuccessCycles(2),
		metrix.WithDescriptorGraceCycles(1),
	)

	retention, ok := store.(metrix.DescriptorRetention)
	if !ok {
		t.Fatal("collector store does not expose descriptor retention")
	}
	if got, want := retention.DescriptorRetentionWindow(), uint64(3); got != want {
		t.Fatalf("descriptor retention window = %d, want %d", got, want)
	}

	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store is not cycle-managed")
	}

	ctrl := managed.CycleController()
	ctrl.BeginCycle()
	meter := managed.Write().SnapshotMeter("contract").WithLabels(metrix.Label{Key: "scope", Value: "api"})
	meter.Gauge("gauge", metrix.WithUnit("items")).Observe(42)
	meter.Counter("total").ObserveTotal(7)
	meter.StateSet(
		"status",
		metrix.WithStateSetStates("down", "up"),
		metrix.WithStateSetMode(metrix.ModeEnum),
	).Enable("up")
	if err := ctrl.CommitCycleSuccess(); err != nil {
		t.Fatalf("commit cycle: %v", err)
	}

	reader := store.Read()
	requireValue(t, reader, "contract.gauge", metrix.Labels{"scope": "api"}, 42)
	requireValue(t, reader, "contract.total", metrix.Labels{"scope": "api"}, 7)
	if got := retention.SuccessfulCommits(); got != 1 {
		t.Fatalf("successful commits = %d, want 1", got)
	}
	meta, ok := reader.MetricMeta("contract.gauge")
	if !ok {
		t.Fatal("metric metadata not found")
	}
	if got, want := meta.Unit, "items"; got != want {
		t.Fatalf("metric unit = %q, want %q", got, want)
	}

	flat := store.Read(metrix.ReadFlatten())
	requireValue(t, flat, "contract.status", metrix.Labels{"scope": "api", "contract.status": "up"}, 1)
	requireValue(t, flat, "contract.status", metrix.Labels{"scope": "api", "contract.status": "down"}, 0)
}

func TestRuntimeStorePublicContract(t *testing.T) {
	store := metrix.NewRuntimeStore()
	meter := store.Write().StatefulMeter("runtime").WithLabels(metrix.Label{Key: "queue", Value: "default"})

	meter.Gauge("depth").Set(3)
	meter.Counter("jobs_total").Add(2)
	meter.Counter("jobs_total").Add(5)

	reader := store.Read()
	requireValue(t, reader, "runtime.depth", metrix.Labels{"queue": "default"}, 3)
	requireValue(t, reader, "runtime.jobs_total", metrix.Labels{"queue": "default"}, 7)
	requireDelta(t, reader, "runtime.jobs_total", metrix.Labels{"queue": "default"}, 5)
}

func requireValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want metrix.SampleValue) {
	t.Helper()
	got, ok := r.Value(name, labels)
	if !ok {
		t.Fatalf("value %q with labels %#v not found", name, labels)
	}
	if got != want {
		t.Fatalf("value %q with labels %#v = %v, want %v", name, labels, got, want)
	}
}

func requireDelta(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want metrix.SampleValue) {
	t.Helper()
	got, ok := r.Delta(name, labels)
	if !ok {
		t.Fatalf("delta %q with labels %#v not found", name, labels)
	}
	if got != want {
		t.Fatalf("delta %q with labels %#v = %v, want %v", name, labels, got, want)
	}
}
