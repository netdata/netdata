package jobmgr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestKernelFunctionResourceLanesGrowAndPreserveCrossLaneProgress(
	t *testing.T,
) {
	const hotLanePopulation = 33
	kernel, run, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)

	require.NoError(t, run.OpenAdmission())

	setTestFunctionResource(
		t,
		kernel,
		func(lookup FunctionLookup) string {
			if strings.HasPrefix(lookup.UID, "hot-") {
				return "hot"
			}
			return "cold"
		},
	)
	for index := range hotLanePopulation {
		request := Request{
			UID:    fmt.Sprintf("hot-%02d", index),
			Source: lifecycle.SourceFunction,
			Route:  "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		require.NoError(t, err)

		require.NoError(t, kernel.admit(request, plan, nil, nil, nil))
	}
	cold := Request{
		UID:    "cold-progress",
		Source: lifecycle.SourceFunction,
		Route:  "route",
	}
	plan, err := kernel.prepareSubmissionPlanForTest(cold)
	require.NoError(t, err)

	require.NoError(t, kernel.admit(cold, plan, nil, nil, nil))
}

func TestKernelReadyLaneFairnessAtBoundaries(t *testing.T) {
	populations := map[string]int{
		"one":                   1,
		"thirty-two":            32,
		"two-hundred-fifty-six": 256,
	}
	for name, population := range populations {
		t.Run(name, func(t *testing.T) {
			kernel, _ := newKernel(t)
			expected := [2][]*commandLane{}
			for sourceIndex, source := range []lifecycle.Source{
				lifecycle.SourceJobManager,
				lifecycle.SourceFunction,
			} {
				for index := range population {
					uid := fmt.Sprintf("%d-%03d", sourceIndex, index)
					generation, err := lifecycle.NewOperation(
						lifecycle.OperationID(index+1),
						uid,
						source,
						uid,
						false,
					)
					require.NoError(t, err)
					operation := &commandOperation{
						OperationGeneration: generation,
					}
					lane := &commandLane{
						key:    uid,
						source: source,
						head:   operation,
					}
					expected[sourceIndex] = append(expected[sourceIndex], lane)
					kernel.markReady(lane)
				}
			}
			positions := [2]int{}
			for decision := 0; decision < 2*population; decision++ {
				lane := kernel.nextReadyLane()
				require.NotNil(t, lane)
				source := sourceIndex(lane.source)
				want := expected[source][positions[source]]
				require.EqualValues(t, want, lane)
				positions[source]++
				kernel.markReady(lane)
			}
			require.EqualValues(t, [2]int{population, population}, positions)
		})
	}
}
