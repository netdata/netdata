package schedulers

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

func TestApplyGetRemove(t *testing.T) {
	def := Definition{Name: "custom", Workers: 25, QueueSize: 64}
	if err := ApplyDefinition(def, logger.New()); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got, ok := Get("custom"); !ok || got.Workers != 25 || got.QueueSize != 64 {
		t.Fatalf("unexpected definition: %+v ok=%v", got, ok)
	}
	if err := RemoveDefinition("custom"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, ok := Get("custom"); ok {
		t.Fatalf("expected custom to be removed")
	}
}

func TestDefaultAlwaysPresent(t *testing.T) {
	ApplyDefinition(Definition{Name: "default", Workers: 10, QueueSize: 10}, logger.New())
	if def, ok := Get("default"); !ok || def.Workers != 10 || def.Builtin {
		t.Fatalf("expected customized default, got %+v ok=%v", def, ok)
	}
	if err := RemoveDefinition("default"); err != nil {
		t.Fatalf("remove default failed: %v", err)
	}
	if def, ok := Get("default"); !ok || def.Workers != 50 || def.QueueSize != 128 || !def.Builtin {
		t.Fatalf("default definition should reset, got %+v ok=%v", def, ok)
	}
}

func TestAttachDetachJob(t *testing.T) {
	def := Definition{Name: "attach", Workers: 5, QueueSize: 16}
	if err := ApplyDefinition(def, logger.New()); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	reg := runtime.JobRegistration{Spec: testJobSpec("job1")}
	handle, err := AttachJob("attach", reg, logger.New())
	if err != nil {
		t.Fatalf("attach failed: %v", err)
	}
	if handle == nil {
		t.Fatalf("handle nil")
	}
	DetachJob(handle)
	if err := RemoveDefinition("attach"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
}

func testJobSpec(name string) spec.JobSpec {
	return spec.JobSpec{
		Name:             name,
		Plugin:           "/bin/true",
		CheckInterval:    time.Second,
		RetryInterval:    time.Second,
		Timeout:          time.Second,
		MaxCheckAttempts: 1,
	}
}
