// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package ddprofiledefinition

import "slices"

type ProfileConsumer string

const (
	ConsumerMetrics  ProfileConsumer = "metrics"
	ConsumerTopology ProfileConsumer = "topology"
)

type ConsumerSet []ProfileConsumer

func (s ConsumerSet) Clone() ConsumerSet {
	return slices.Clone(s)
}

func (s ConsumerSet) Contains(consumer ProfileConsumer) bool {
	for _, c := range s {
		if c == consumer {
			return true
		}
	}
	return false
}

func (s ConsumerSet) IsEmpty() bool {
	return len(s) == 0
}
