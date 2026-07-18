package agentdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/driverkit"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestPerformanceEventObserverForwardsExactPassiveEvent(t *testing.T) {
	var wire bytes.Buffer
	emitter, err := driverkit.NewEmitter(&wire, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := (eventObserver{emitter: emitter}).Observe(perffixture.EventDeadlineObserved, "u1", "perf:work-007"); err != nil {
		t.Fatal(err)
	}
	event, err := protocol.ReadEvent(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != protocol.EventDeadlineObserved || event.Token != "u1" || event.RouteKey != "perf:work-007" || len(event.Payload) != 0 {
		t.Fatalf("current-real passive event differs: %#v", event)
	}
}

func TestPerformanceCreatorCopiesCompleteNeutralDeclaration(t *testing.T) {
	creator := performanceCreator(nil)
	if !creator.FunctionOnly || creator.Create == nil || creator.AgentFunctions == nil || creator.MethodHandler == nil {
		t.Fatalf("creator is incomplete: %#v", creator)
	}
	functions := creator.AgentFunctions()
	if len(functions) != perffixture.FunctionCount || functions[0].ID != "work-000" || functions[0].FunctionName != "perf:work-000" ||
		functions[255].ID != "work-255" || functions[255].FunctionName != "perf:work-255" {
		t.Fatalf("function adapter differs: first=%#v last=%#v count=%d", functions[0], functions[len(functions)-1], len(functions))
	}
	collector := creator.Create()
	if err := collector.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(collector.Configuration())
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"option_str":"work","option_int":1}` {
		t.Fatalf("config JSON differs: %s", payload)
	}
}

func TestPerformanceHandlerAdaptsSemanticOutcomesToCurrentWireValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		mode       string
		context    func() (context.Context, context.CancelFunc)
		observer   perffixture.Observer
		want       string
		wantLength int
	}{
		{name: "function", mode: "F", context: backgroundContext, want: `{"status":200}`, wantLength: 14},
		{name: "capacity", mode: "K", context: backgroundContext, wantLength: 4_095},
		{name: "deadline", mode: "D", context: immediateDeadline, observer: noOpObserver{}, want: `{"errorMessage":"Deadline exceeded.","status":504}`, wantLength: 50},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.context()
			defer cancel()
			handler := &performanceHandler{observer: test.observer}
			response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "work-007", Args: []string{"mode:" + test.mode, "token:u1"}})
			payload, err := json.Marshal(response.RawResponse)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) != test.wantLength {
				t.Fatalf("payload length=%d, want %d: %s", len(payload), test.wantLength, payload)
			}
			if test.want != "" && string(payload) != test.want {
				t.Fatalf("payload differs: %s", payload)
			}
			if test.mode == "K" && (!strings.HasPrefix(string(payload), `{"pad":"`) || !strings.HasSuffix(string(payload), `","status":200}`)) {
				t.Fatalf("capacity payload differs: %s", payload)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	handler := &performanceHandler{observer: cancellingObserver{cancel: cancel}}
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "work-008", Args: []string{"mode:C", "token:u2"}})
	payload, err := json.Marshal(response.RawResponse)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"errorMessage":"Request cancelled.","status":499}` || len(payload) != 50 {
		t.Fatalf("cancel payload differs: %s", payload)
	}
}

func backgroundContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func immediateDeadline() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 0)
}

type noOpObserver struct{}

func (noOpObserver) Observe(perffixture.Event, string, string) error { return nil }

type cancellingObserver struct {
	cancel context.CancelFunc
}

func (observer cancellingObserver) Observe(perffixture.Event, string, string) error {
	observer.cancel()
	return nil
}
