package perffixture

import (
	"context"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

func TestDeclarationContainsExactFunctionsAndConfigs(t *testing.T) {
	functions := Functions()
	if len(functions) != FunctionCount || functions[0].ID != "work-000" || functions[0].PublicName != "perf:work-000" ||
		functions[255].ID != "work-255" || functions[255].PublicName != "perf:work-255" {
		t.Fatalf("function declaration differs: first=%#v last=%#v count=%d", functions[0], functions[len(functions)-1], len(functions))
	}
	var file struct {
		Jobs []struct {
			Name   string `yaml:"name"`
			Config `yaml:",inline"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(ConfigYAML(), &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Jobs) != FunctionCount || file.Jobs[0].Name != "job-000" || file.Jobs[255].Name != "job-255" {
		t.Fatalf("config declaration differs: first=%#v last=%#v count=%d", file.Jobs[0], file.Jobs[len(file.Jobs)-1], len(file.Jobs))
	}
	for _, job := range file.Jobs {
		if job.Config != DefaultConfig() {
			t.Fatalf("job %s config differs: %#v", job.Name, job.Config)
		}
	}
}

func TestExecuteCoversSuccessCapacityCancelAndDeadline(t *testing.T) {
	for _, test := range []struct {
		mode string
		kind OutcomeKind
		pad  int
	}{
		{mode: "F", kind: OutcomeSuccess},
		{mode: "K", kind: OutcomeSuccess, pad: CapacityPad},
	} {
		outcome, err := Execute(context.Background(), "work-007", []string{"mode:" + test.mode, "token:u1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Kind != test.kind || outcome.PadLength != test.pad {
			t.Fatalf("mode %s outcome differs: %#v", test.mode, outcome)
		}
	}

	observer := &recordingObserver{events: make(chan observedEvent, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan Outcome, 1)
	go func() {
		outcome, _ := Execute(ctx, "work-008", []string{"mode:C", "token:u2"}, observer)
		cancelled <- outcome
	}()
	select {
	case event := <-observer.events:
		if event.kind != EventHandlerEntered || event.token != "u2" || event.route != "perf:work-008" {
			t.Fatalf("cancel event differs: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("handler-entered was not observed")
	}
	cancel()
	if outcome := <-cancelled; outcome.Kind != OutcomeCancelled {
		t.Fatalf("cancel outcome differs: %#v", outcome)
	}

	deadlineCtx, stop := context.WithTimeout(context.Background(), 0)
	defer stop()
	outcome, err := Execute(deadlineCtx, "work-009", []string{"mode:D", "token:u3"}, observer)
	if err != nil || outcome.Kind != OutcomeDeadline {
		t.Fatalf("deadline outcome differs: %#v err=%v", outcome, err)
	}
	if event := <-observer.events; event.kind != EventDeadlineObserved || event.token != "u3" || event.route != "perf:work-009" {
		t.Fatalf("deadline event differs: %#v", event)
	}
}

func TestExecuteRejectsMalformedPublicInput(t *testing.T) {
	for _, test := range []struct {
		function string
		args     []string
	}{
		{function: "work-256", args: []string{"mode:F", "token:u1"}},
		{function: "work-001", args: []string{"mode:F"}},
		{function: "work-001", args: []string{"mode:X", "token:u1"}},
		{function: "work-001", args: []string{"mode:F", "token:bad value"}},
	} {
		if _, err := Execute(context.Background(), test.function, test.args, nil); err == nil || !strings.Contains(err.Error(), "performance fixture") {
			t.Fatalf("malformed input accepted: %#v", test)
		}
	}
}

type recordingObserver struct {
	events chan observedEvent
}

type observedEvent struct {
	kind  Event
	token string
	route string
}

func (observer *recordingObserver) Observe(event Event, token, route string) error {
	observer.events <- observedEvent{kind: event, token: token, route: route}
	return nil
}
