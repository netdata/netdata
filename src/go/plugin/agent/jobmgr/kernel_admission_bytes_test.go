// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var operationAdmissionBytesBenchmarkSink int64

func TestOperationAdmissionBytesDoesNotAllocate(t *testing.T) {
	request, plan := populatedOperationAdmissionFixture()
	bytes, err := operationAdmissionBytes(request, plan)
	require.NoError(t, err)
	require.EqualValues(t, 5_212, bytes)

	allocations := testing.AllocsPerRun(1_000, func() {
		got, err := operationAdmissionBytes(request, plan)
		if err != nil {
			panic(err)
		}
		operationAdmissionBytesBenchmarkSink = got
	})

	require.Zero(t, allocations)
}

func BenchmarkOperationAdmissionBytes(b *testing.B) {
	request, plan := populatedOperationAdmissionFixture()
	b.ReportAllocs()

	for b.Loop() {
		got, err := operationAdmissionBytes(request, plan)
		if err != nil {
			b.Fatal(err)
		}
		operationAdmissionBytesBenchmarkSink = got
	}
}

func populatedOperationAdmissionFixture() (Request, WorkPlan) {
	return Request{
			UID:          "admission-uid",
			LaneKey:      "module_job",
			Route:        "dyncfg/job/update",
			ContentType:  "application/json",
			Permissions:  "signed-id",
			CallerSource: "cloud",
			Args:         []string{"dyncfg", "update", "module:job", "source=cloud"},
		}, WorkPlan{
			Claims: []string{
				"dyncfg:jobs", "job:module_job", "secrets:store-a", "vnode:host-a",
			},
		}
}
