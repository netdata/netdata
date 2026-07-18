package perffixture

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	ModuleName    = "perf"
	FunctionCount = 256
	CapacityPad   = 4_072
)

type Config struct {
	OptionStr string `yaml:"option_str" json:"option_str"`
	OptionInt int    `yaml:"option_int" json:"option_int"`
}

type Function struct {
	ID         string
	PublicName string
}

type Event string

const (
	EventHandlerEntered   Event = "handler-entered"
	EventDeadlineObserved Event = "deadline-observed"
)

type Observer interface {
	Observe(event Event, token, route string) error
}

type OutcomeKind uint8

const (
	OutcomeSuccess OutcomeKind = iota + 1
	OutcomeCancelled
	OutcomeDeadline
)

type Outcome struct {
	Kind      OutcomeKind
	PadLength int
}

func DefaultConfig() Config {
	return Config{OptionStr: "work", OptionInt: 1}
}

func Functions() []Function {
	functions := make([]Function, FunctionCount)
	for ordinal := range FunctionCount {
		key := fmt.Sprintf("%03d", ordinal)
		functions[ordinal] = Function{ID: "work-" + key, PublicName: "perf:work-" + key}
	}
	return functions
}

func ConfigYAML() []byte {
	var builder strings.Builder
	builder.Grow(18 * 1024)
	builder.WriteString("jobs:\n")
	for ordinal := range FunctionCount {
		fmt.Fprintf(&builder, "  - name: job-%03d\n    option_str: work\n    option_int: 1\n", ordinal)
	}
	return []byte(builder.String())
}

func Execute(ctx context.Context, functionID string, args []string, observer Observer) (Outcome, error) {
	if !validFunctionID(functionID) {
		return Outcome{}, fmt.Errorf("performance fixture: invalid function %q", functionID)
	}
	mode, token, err := parseArgs(args)
	if err != nil {
		return Outcome{}, err
	}
	route := ModuleName + ":" + functionID
	switch mode {
	case 'F':
		return Outcome{Kind: OutcomeSuccess}, nil
	case 'K':
		return Outcome{Kind: OutcomeSuccess, PadLength: CapacityPad}, nil
	case 'C':
		if observer == nil {
			return Outcome{}, errors.New("performance fixture: cancel observer is required")
		}
		if err := observer.Observe(EventHandlerEntered, token, route); err != nil {
			return Outcome{}, err
		}
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.Canceled) {
			return Outcome{}, fmt.Errorf("performance fixture: cancel mode observed %v", ctx.Err())
		}
		return Outcome{Kind: OutcomeCancelled}, nil
	case 'D':
		if observer == nil {
			return Outcome{}, errors.New("performance fixture: deadline observer is required")
		}
		if _, ok := ctx.Deadline(); !ok {
			return Outcome{}, errors.New("performance fixture: deadline mode has no context deadline")
		}
		<-ctx.Done()
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Outcome{}, fmt.Errorf("performance fixture: deadline mode context error %v", ctx.Err())
		}
		if !errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			return Outcome{}, fmt.Errorf("performance fixture: deadline mode observed %v", context.Cause(ctx))
		}
		if err := observer.Observe(EventDeadlineObserved, token, route); err != nil {
			return Outcome{}, err
		}
		return Outcome{Kind: OutcomeDeadline}, nil
	default:
		return Outcome{}, fmt.Errorf("performance fixture: invalid mode %q", mode)
	}
}

func validFunctionID(functionID string) bool {
	if len(functionID) != len("work-000") || !strings.HasPrefix(functionID, "work-") {
		return false
	}
	ordinal, err := strconv.Atoi(functionID[len("work-"):])
	return err == nil && ordinal >= 0 && ordinal < FunctionCount
}

func parseArgs(args []string) (byte, string, error) {
	if len(args) != 2 || len(args[0]) != len("mode:X") || !strings.HasPrefix(args[0], "mode:") || !strings.HasPrefix(args[1], "token:") {
		return 0, "", errors.New("performance fixture: expected mode:X and token:value")
	}
	mode := args[0][len("mode:")]
	token := strings.TrimPrefix(args[1], "token:")
	if token == "" || strings.ContainsAny(token, " \t\r\n\x00") {
		return 0, "", errors.New("performance fixture: invalid token")
	}
	return mode, token, nil
}
