package agentdriver

import (
	"context"
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/driverkit"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func performanceCreator(emitter *driverkit.Emitter) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults:     collectorapi.Defaults{UpdateEvery: 1, Priority: collectorapi.Priority},
		Create:       func() collectorapi.CollectorV1 { return newPerformanceCollector() },
		Config:       func() any { config := perffixture.DefaultConfig(); return &config },
		FunctionOnly: true,
		AgentFunctions: func() []funcapi.FunctionConfig {
			declared := perffixture.Functions()
			functions := make([]funcapi.FunctionConfig, len(declared))
			for index, function := range declared {
				functions[index] = funcapi.FunctionConfig{
					ID: function.ID, FunctionName: function.PublicName, Name: function.PublicName,
					UpdateEvery: 1, RawRequest: true,
				}
			}
			return functions
		},
		MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &performanceHandler{observer: eventObserver{emitter: emitter}}
		},
	}
}

type performanceCollector struct {
	collectorapi.Base
	perffixture.Config `yaml:",inline" json:""`
}

func newPerformanceCollector() *performanceCollector {
	return &performanceCollector{Config: perffixture.DefaultConfig()}
}

func (collector *performanceCollector) Configuration() any                       { return collector.Config }
func (collector *performanceCollector) Charts() *collectorapi.Charts             { return nil }
func (collector *performanceCollector) Collect(context.Context) map[string]int64 { return nil }
func (collector *performanceCollector) Cleanup(context.Context)                  {}

func (collector *performanceCollector) Init(context.Context) error {
	if collector.Config != perffixture.DefaultConfig() {
		return errors.New("performance fixture config differs from immutable declaration")
	}
	return nil
}

func (collector *performanceCollector) Check(context.Context) error { return nil }

type performanceHandler struct {
	observer perffixture.Observer
}

func (handler *performanceHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (handler *performanceHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.InternalErrorResponse("raw performance request required")
}

func (handler *performanceHandler) HandleRaw(ctx context.Context, request funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	outcome, err := perffixture.Execute(ctx, request.Method, request.Args, handler.observer)
	if err != nil {
		return funcapi.RawResponse(map[string]any{"errorMessage": err.Error(), "status": 500})
	}
	switch outcome.Kind {
	case perffixture.OutcomeSuccess:
		response := map[string]any{"status": 200}
		if outcome.PadLength > 0 {
			response["pad"] = strings.Repeat("A", outcome.PadLength)
		}
		return funcapi.RawResponse(response)
	case perffixture.OutcomeCancelled:
		return funcapi.RawResponse(map[string]any{"errorMessage": "Request cancelled.", "status": 499})
	case perffixture.OutcomeDeadline:
		return funcapi.RawResponse(map[string]any{"errorMessage": "Deadline exceeded.", "status": 504})
	default:
		return funcapi.RawResponse(map[string]any{"errorMessage": "Invalid fixture outcome.", "status": 500})
	}
}

func (handler *performanceHandler) Cleanup(context.Context) {}

type eventObserver struct {
	emitter *driverkit.Emitter
}

func (observer eventObserver) Observe(event perffixture.Event, token, route string) error {
	if observer.emitter == nil {
		return errors.New("current-real event emitter is unavailable")
	}
	var kind protocol.EventKind
	switch event {
	case perffixture.EventHandlerEntered:
		kind = protocol.EventHandlerEntered
	case perffixture.EventDeadlineObserved:
		kind = protocol.EventDeadlineObserved
	default:
		return errors.New("current-real performance event is unsupported")
	}
	return observer.emitter.Event(kind, token, route, nil)
}
