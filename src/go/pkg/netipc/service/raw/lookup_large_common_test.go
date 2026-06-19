package raw

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	lookupTopologyScaleItems       = 8192
	lookupHPCScaleItems            = 32768
	lookupScaleRequestPayloadBytes = 8192
	lookupScaleCallTimeoutMs       = 120000

	lookupResponseSplitScaleItems          = 512
	lookupResponseSplitRequestPayloadBytes = 65536
	lookupResponseSplitPayloadBytes        = 98304
	lookupResponseSplitMinCalls            = 2
	lookupResponseSplitLabelBytes          = 512
)

var (
	lookupResponseSplitLabelKey   = []byte("scale")
	lookupResponseSplitLabelValue = bytes.Repeat([]byte("l"), lookupResponseSplitLabelBytes)
)

func largeLookupPids(count int) []uint32 {
	pids := make([]uint32, count)
	var pid uint32 = 100000
	for i := range count {
		pids[i] = pid
		pid++
	}
	return pids
}

func largeLookupU32(value int) uint32 {
	return uint32(value) // #nosec G115 -- large lookup tests only pass bounded 8192/32768 cardinalities.
}

func largeLookupPaths(count int) [][]byte {
	paths := make([][]byte, count)
	for i := range count {
		paths[i] = fmt.Appendf(nil, "/cg/%05d", i)
	}
	return paths
}

func recordLookupFragment(maxSeen *atomic.Uint32, itemCount uint32) {
	for {
		current := maxSeen.Load()
		if itemCount <= current || maxSeen.CompareAndSwap(current, itemCount) {
			return
		}
	}
}

func responseSplitAppsLookupHandler(calls, maxSeen *atomic.Uint32) AppsLookupHandler {
	labels := []struct{ Key, Value []byte }{
		{Key: lookupResponseSplitLabelKey, Value: lookupResponseSplitLabelValue},
	}
	return func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
		calls.Add(1)
		recordLookupFragment(maxSeen, req.ItemCount)
		builder.SetGeneration(9)
		for i := uint32(0); i < req.ItemCount; i++ {
			pid, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				pid,
				1,
				1000,
				42,
				[]byte("ok"),
				[]byte("/ok"),
				[]byte("name"),
				labels,
			); err != nil {
				return false
			}
		}
		return true
	}
}

func responseSplitCgroupsLookupHandler(calls, maxSeen *atomic.Uint32) CgroupsLookupHandler {
	labels := []struct{ Key, Value []byte }{
		{Key: lookupResponseSplitLabelKey, Value: lookupResponseSplitLabelValue},
	}
	return func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
		calls.Add(1)
		recordLookupFragment(maxSeen, req.ItemCount)
		builder.SetGeneration(7)
		for i := uint32(0); i < req.ItemCount; i++ {
			path, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				path.Bytes(),
				[]byte("ok"),
				labels,
			); err != nil {
				return false
			}
		}
		return true
	}
}

func verifyResponseSplitAppsLookupResponse(t *testing.T, view *protocol.AppsLookupResponseView, pids []uint32) {
	t.Helper()
	if view.ItemCount != largeLookupU32(len(pids)) || view.Generation != 9 {
		t.Fatalf("apps response-split header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, expected := range pids {
		item, err := view.Item(largeLookupU32(i))
		if err != nil {
			t.Fatalf("apps response-split item %d decode: %v", i, err)
		}
		if item.Status != protocol.PidLookupKnown ||
			item.Pid != expected ||
			item.Comm.String() != "ok" ||
			item.CgroupPath.String() != "/ok" ||
			item.LabelCount != 1 {
			t.Fatalf("apps response-split item %d = %+v, want pid %d known with one label", i, item, expected)
		}
		label, err := item.Label(0)
		if err != nil {
			t.Fatalf("apps response-split item %d label decode: %v", i, err)
		}
		if !bytes.Equal(label.Key.Bytes(), lookupResponseSplitLabelKey) ||
			!bytes.Equal(label.Value.Bytes(), lookupResponseSplitLabelValue) {
			t.Fatalf("apps response-split item %d label = %q/%d bytes", i, label.Key.String(), label.Value.Len())
		}
	}
}

func verifyResponseSplitCgroupsLookupResponse(t *testing.T, view *protocol.CgroupsLookupResponseView, paths [][]byte) {
	t.Helper()
	if view.ItemCount != largeLookupU32(len(paths)) || view.Generation != 7 {
		t.Fatalf("cgroups response-split header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, expected := range paths {
		item, err := view.Item(largeLookupU32(i))
		if err != nil {
			t.Fatalf("cgroups response-split item %d decode: %v", i, err)
		}
		if item.Status != protocol.CgroupLookupKnown ||
			item.Path.String() != string(expected) ||
			item.Name.String() != "ok" ||
			item.LabelCount != 1 {
			t.Fatalf("cgroups response-split item %d = %+v, want path %q known with one label", i, item, expected)
		}
		label, err := item.Label(0)
		if err != nil {
			t.Fatalf("cgroups response-split item %d label decode: %v", i, err)
		}
		if !bytes.Equal(label.Key.Bytes(), lookupResponseSplitLabelKey) ||
			!bytes.Equal(label.Value.Bytes(), lookupResponseSplitLabelValue) {
			t.Fatalf("cgroups response-split item %d label = %q/%d bytes", i, label.Key.String(), label.Value.Len())
		}
	}
}

func largeAppsLookupHandler(calls, maxSeen *atomic.Uint32) AppsLookupHandler {
	return func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
		calls.Add(1)
		recordLookupFragment(maxSeen, req.ItemCount)
		builder.SetGeneration(9)
		for i := uint32(0); i < req.ItemCount; i++ {
			pid, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				pid,
				1,
				1000,
				42,
				[]byte("ok"),
				[]byte("/ok"),
				[]byte("name"),
				nil,
			); err != nil {
				return false
			}
		}
		return true
	}
}

func largeCgroupsLookupHandler(calls, maxSeen *atomic.Uint32) CgroupsLookupHandler {
	return func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
		calls.Add(1)
		recordLookupFragment(maxSeen, req.ItemCount)
		builder.SetGeneration(7)
		for i := uint32(0); i < req.ItemCount; i++ {
			path, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				path.Bytes(),
				[]byte("ok"),
				nil,
			); err != nil {
				return false
			}
		}
		return true
	}
}

func verifyLargeAppsLookupResponse(t *testing.T, view *protocol.AppsLookupResponseView, pids []uint32) {
	t.Helper()
	if view.ItemCount != largeLookupU32(len(pids)) || view.Generation != 9 {
		t.Fatalf("apps large header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, expected := range pids {
		item, err := view.Item(largeLookupU32(i))
		if err != nil {
			t.Fatalf("apps large item %d decode: %v", i, err)
		}
		if item.Status != protocol.PidLookupKnown ||
			item.Pid != expected ||
			item.Comm.String() != "ok" ||
			item.CgroupPath.String() != "/ok" {
			t.Fatalf("apps large item %d = %+v, want pid %d known", i, item, expected)
		}
	}
}

func verifyLargeCgroupsLookupResponse(t *testing.T, view *protocol.CgroupsLookupResponseView, paths [][]byte) {
	t.Helper()
	if view.ItemCount != largeLookupU32(len(paths)) || view.Generation != 7 {
		t.Fatalf("cgroups large header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, expected := range paths {
		item, err := view.Item(largeLookupU32(i))
		if err != nil {
			t.Fatalf("cgroups large item %d decode: %v", i, err)
		}
		if item.Status != protocol.CgroupLookupKnown ||
			item.Path.String() != string(expected) ||
			item.Name.String() != "ok" {
			t.Fatalf("cgroups large item %d = %+v, want path %q known", i, item, expected)
		}
	}
}
