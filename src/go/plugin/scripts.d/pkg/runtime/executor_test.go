package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

func TestExecutorSingleFlight(t *testing.T) {
	work := func(ctx context.Context, job JobRuntime) ExecutionResult {
		start := time.Now()
		time.Sleep(5 * time.Millisecond)
		return ExecutionResult{Job: job, Start: start, End: time.Now(), Duration: time.Since(start)}
	}

	exec, err := NewExecutor(ExecutorConfig{
		Workers:       1,
		QueueCapacity: 2,
		Work:          work,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec.Start(ctx)
	defer exec.Stop()

	job := JobRuntime{ID: "job-1", Spec: spec.JobSpec{Name: "job-1"}}

	if ok, err := exec.Enqueue(job); err != nil || !ok {
		t.Fatalf("expected first enqueue to succeed, got ok=%v err=%v", ok, err)
	}

	if ok, err := exec.Enqueue(job); err != nil {
		t.Fatalf("unexpected error on duplicate enqueue: %v", err)
	} else if ok {
		t.Fatalf("expected duplicate enqueue to be skipped")
	}

	select {
	case <-exec.Results():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for execution result")
	}

	if ok, err := exec.Enqueue(job); err != nil || !ok {
		t.Fatalf("expected enqueue after completion to succeed, got ok=%v err=%v", ok, err)
	}
}
