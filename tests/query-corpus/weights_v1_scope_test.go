package corpus

import (
	"fmt"
	"reflect"
	"testing"
)

func TestWeightsV1HostScope(t *testing.T) {
	trackContract(t, "W/v1-host-scope")
	const context = "fixture.weights_v1_scope"
	for i := 0; i < 2; i++ {
		ch := weightsFixture()
		ch.ID, ch.Context = context, context
		ch.Dimensions = ch.Dimensions[:1]
		ch.Dimensions[0].ID = fmt.Sprintf("host%d", i)
		weightsSettle(t, fmt.Sprintf("weights-scope-%d", i), guid(9800+i), ch)
	}
	for i := 0; i < 2; i++ {
		for _, endpoint := range []string{"api/v1/weights"} {
			p := weightsV1Params("value", context, "raw", false)
			doc, err := td.HostJSON(fmt.Sprintf("weights-scope-%d", i), endpoint, p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeV1ContextsWeights(doc, context)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]float64{fmt.Sprintf("host%d", i): 50}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s host %d: got %v, want %v", endpoint, i, got, want)
			}
		}
	}
	for _, version := range []string{"v2", "v3"} {
		p := weightsV1Params("value", "", "raw", false)
		p.Set("scope_contexts", context)
		p.Set("scope_nodes", "weights-scope-0|weights-scope-1")
		doc, err := td.HostJSON("weights-scope-0", "api/"+version+"/weights", p)
		if err != nil {
			t.Fatal(err)
		}
		rows := weightsLimitRows(t, doc)
		if len(rows) != 2 || rows["host0"] == nil || rows["host1"] == nil {
			t.Errorf("%s lost multi-node scope: %v", version, rows)
		}
	}
}
