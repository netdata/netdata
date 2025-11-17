package zabbix

import (
	"errors"
	"fmt"
	"testing"
	"time"

	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	pkgzabbix "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
)

func TestBuildStateChartDims(t *testing.T) {
	dims, ids := buildStateChartDims("zabbix.job.state")
	if len(dims) != len(stateDimensions) {
		t.Fatalf("expected %d dims, got %d", len(stateDimensions), len(dims))
	}
	for _, name := range stateDimensions {
		id, ok := ids[name]
		if !ok || id == "" {
			t.Fatalf("missing dim mapping for %s", name)
		}
	}
}

func TestJobStateChart(t *testing.T) {
	chart := buildJobStateChart(pkgzabbix.JobConfig{Name: "fs_usage"})
	if chart == nil || chart.ID == "" {
		t.Fatalf("expected job state chart")
	}
	if len(chart.Dims) != len(stateDimensions) {
		t.Fatalf("expected %d dims, got %d", len(stateDimensions), len(chart.Dims))
	}
	if len(chart.Labels) == 0 {
		t.Fatalf("expected labels on job state chart")
	}
}

func TestInstanceStateChart(t *testing.T) {
	inst := &instanceBinding{id: "instance1", macros: map[string]string{"{#INSTANCE_ID}": "instance1"}}
	chart := buildInstanceStateChart(pkgzabbix.JobConfig{Name: "fs_usage"}, inst)
	if chart == nil || chart.ID == "" {
		t.Fatalf("expected instance state chart")
	}
	if len(chart.Dims) != len(stateDimensions) {
		t.Fatalf("expected %d dims, got %d", len(stateDimensions), len(chart.Dims))
	}
	if len(chart.Labels) == 0 {
		t.Fatalf("expected labels on instance state chart")
	}
}

func TestPipelineEmitterCollectFailureState(t *testing.T) {
	job := testJobConfig()
	emitter := newTestEmitter(job)
	jr := runtime.JobRuntime{ID: "job1", Spec: spec.JobSpec{Name: job.Name}}
	res := runtime.ExecutionResult{
		Job:      jr,
		Err:      errors.New("boom"),
		ExitCode: 2,
		End:      time.Now(),
	}
	emitter.Emit(jr, res, runtime.JobSnapshot{})
	metrics := emitter.Flush()
	assertState(t, metrics, job, "collect_failure", 1)
	assertState(t, metrics, job, "ok", 0)
	assertInstanceState(t, metrics, job, "default", "collect_failure", 1)
	assertInstanceState(t, metrics, job, "default", "ok", 0)
}

func TestPipelineEmitterExtractionFailureState(t *testing.T) {
	job := testJobConfig()
	emitter := newTestEmitter(job)
	jr := runtime.JobRuntime{ID: "job2", Spec: spec.JobSpec{Name: job.Name}}
	res := runtime.ExecutionResult{
		Job:    jr,
		Output: []byte("not-json"),
		End:    time.Now(),
	}
	emitter.Emit(jr, res, runtime.JobSnapshot{})
	metrics := emitter.Flush()
	assertState(t, metrics, job, "extraction_failure", 1)
	assertState(t, metrics, job, "ok", 0)
	assertInstanceState(t, metrics, job, "default", "extraction_failure", 1)
	assertInstanceState(t, metrics, job, "default", "ok", 0)
}

func newTestEmitter(job pkgzabbix.JobConfig) *pipelineEmitter {
	charts := &module.Charts{}
	proc := zpre.NewPreprocessor("test-shard")
	emitter, err := newPipelineEmitter(nil, proc, charts, []pkgzabbix.JobConfig{job})
	if err != nil {
		panic(err)
	}
	return emitter
}

func testJobConfig() pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name: "fs_usage",
		Collection: pkgzabbix.CollectionConfig{
			Type:    pkgzabbix.CollectionCommand,
			Command: "/usr/bin/true",
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "value",
				Context:   "zabbix.fs_usage.value",
				Dimension: "value",
				Unit:      "value",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.value"},
				},
			},
		},
	}
}

func assertState(t *testing.T, metrics map[string]int64, job pkgzabbix.JobConfig, dim string, want int64) {
	chartID := jobStateChartID(job)
	key := fmt.Sprintf("%s.%s", chartID, ids.Sanitize(dim))
	if got := metrics[key]; got != want {
		t.Fatalf("job state %s expected %d, got %d", dim, want, got)
	}
}

func assertInstanceState(t *testing.T, metrics map[string]int64, job pkgzabbix.JobConfig, instID, dim string, want int64) {
	inst := &instanceBinding{id: instID}
	chartID := instanceStateChartID(job, inst)
	key := fmt.Sprintf("%s.%s", chartID, ids.Sanitize(dim))
	if got := metrics[key]; got != want {
		t.Fatalf("instance %s state %s expected %d, got %d", instID, dim, want, got)
	}
}
