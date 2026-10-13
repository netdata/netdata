// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/stretchr/testify/require"
)

func TestBoundedErrorAccumulatorPreservesControlFlowCauses(t *testing.T) {
	var failures boundedErrorAccumulator
	for range 100_000 {
		failures.Add(errors.New("malformed member"))
	}
	failures.Add(fmt.Errorf("%w: collision", identity.ErrIntegrity))
	failures.Add(context.DeadlineExceeded)

	err := failures.Err()
	require.Error(t, err)
	require.ErrorIs(t, err, identity.ErrIntegrity)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, len(err.Error()), 2048)
	require.Contains(t, err.Error(), "100002 Redfish operation failures")
}

func TestEmbeddedMemberFailuresKeepErrorOutputBounded(t *testing.T) {
	values := make([]any, 100_000)
	for index := range values {
		values[index] = strings.Repeat("invalid", 8)
	}
	client := &Client{}
	nodes, complete, err := client.acquireEmbeddedValues(
		fixtureParent(),
		graphRelationship{
			Path:      "Sensors",
			ChildKind: "sensor",
		},
		values,
	)

	require.Error(t, err)
	require.Empty(t, nodes)
	require.False(t, complete)
	require.Equal(t, "protocol", classifyError(err))
	require.Less(t, len(err.Error()), 2048)
	require.Contains(t, err.Error(), "100000 Redfish operation failures")
}

func TestCollectionDiagnosticsDeduplicateAndRemainCountBounded(t *testing.T) {
	graph := &resourceGraph{}
	graph.addDiagnostic("duplicate")
	graph.addDiagnostic("duplicate")
	for index := range maxCollectionDiagnostics + 10 {
		graph.addDiagnostic(fmt.Sprintf("diagnostic-%d", index))
	}

	diagnostics := graph.diagnostics.Values()
	require.Len(t, diagnostics, maxCollectionDiagnostics+1)
	require.Equal(t, "duplicate", diagnostics[0])
	require.Contains(t, diagnostics[len(diagnostics)-1], "11 additional Redfish collection diagnostics")
}
