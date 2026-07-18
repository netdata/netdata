package jobmgr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestKernelFunctionLaneGrowsAndPreservesCrossLaneProgress(
	t *testing.T,
) {
	const hotLanePopulation = 33
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	setTestFunctionLane(
		t,
		kernel,
		func(lookup FunctionLookup) string {
			if strings.HasPrefix(lookup.UID, "hot-") {
				return "hot"
			}
			return "cold"
		},
	)
	for index := 0; index < hotLanePopulation; index++ {
		request := Request{
			UID:    fmt.Sprintf("hot-%02d", index),
			Source: lifecycle.SourceFunction,
			Route:  "route",
		}
		plan, err := kernel.prepareSubmissionPlanForTest(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := kernel.admit(
			request,
			plan,
			nil,
			nil,
			nil,
		); err != nil {
			t.Fatalf("hot lane admission %d: %v", index, err)
		}
	}
	cold := Request{
		UID:    "cold-progress",
		Source: lifecycle.SourceFunction,
		Route:  "route",
	}
	plan, err := kernel.prepareSubmissionPlanForTest(cold)
	if err != nil {
		t.Fatal(err)
	}
	if err := kernel.admit(
		cold,
		plan,
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf(
			"cold lane was blocked by hot-lane depth: %v",
			err,
		)
	}
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
				for index := 0; index < population; index++ {
					uid := fmt.Sprintf(
						"%d-%03d",
						sourceIndex,
						index,
					)
					generation, err := lifecycle.NewOperation(
						lifecycle.OperationID(index+1),
						uid,
						source,
						uid,
						false,
					)
					if err != nil {
						t.Fatal(err)
					}
					operation := &commandOperation{
						OperationGeneration: generation,
						admitted:            true,
					}
					lane := &commandLane{
						key:    uid,
						source: source,
						head:   operation,
					}
					expected[sourceIndex] = append(
						expected[sourceIndex],
						lane,
					)
					kernel.markReady(lane)
				}
			}
			positions := [2]int{}
			for decision := 0; decision < 2*population; decision++ {
				lane := kernel.nextReadyLane()
				if lane == nil {
					t.Fatalf(
						"decision %d returned no lane",
						decision,
					)
				}
				source := sourceIndex(lane.source)
				want := expected[source][positions[source]]
				if lane != want {
					t.Fatalf(
						"source %d decision %d lane=%s want=%s",
						source,
						positions[source],
						lane.key,
						want.key,
					)
				}
				positions[source]++
				kernel.markReady(lane)
			}
			if positions != [2]int{population, population} {
				t.Fatalf(
					"source decisions=%v want=%d/%d",
					positions,
					population,
					population,
				)
			}
		})
	}
}
