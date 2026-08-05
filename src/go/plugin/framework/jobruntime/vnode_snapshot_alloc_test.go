package jobruntime

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

func TestUnchangedJobVnodeRevisionDoesNotAllocate(t *testing.T) {
	snapshot := VnodeSnapshot{
		Vnode: &vnodes.VirtualNode{
			Name:     "node",
			Hostname: "node",
			GUID:     "11111111-1111-1111-1111-111111111111",
			Labels:   map[string]string{"site": "test"},
		},
		Revision:         1,
		MetadataRevision: 1,
	}
	v1 := &Job{vnodeRevision: 1}
	v2 := &JobV2{vnodeRevision: 1}
	tests := map[string]struct {
		apply func()
	}{
		"V1": {
			apply: func() {
				v1.applyVnodeSnapshot(snapshot)
			},
		},
		"V2": {
			apply: func() {
				v2.applyVnodeSnapshot(snapshot)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			allocations := testing.AllocsPerRun(100, test.apply)
			if allocations != 0 {
				t.Fatalf(
					"unchanged vnode revision allocated %.1f objects",
					allocations,
				)
			}
		})
	}
}
