package zabbix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

func TestBuildJobSpecsCommandPlugin(t *testing.T) {
	cfg := JobConfig{
		Name:      "cmd_job",
		Scheduler: "default",
		Collection: CollectionConfig{
			Type:    CollectionCommand,
			Command: "/usr/libexec/zabbix/collector",
		},
	}
	cfg.Pipelines = []PipelineConfig{{
		Name:      "usage",
		Context:   "zabbix.cmd.usage",
		Dimension: "value",
		Unit:      "%",
		Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
	}}
	specs, err := buildJobSpecs([]JobConfig{cfg})
	if err != nil {
		t.Fatalf("buildJobSpecs returned error: %v", err)
	}
	if specs[0].Plugin != cfg.Collection.Command {
		t.Fatalf("expected plugin %q, got %q", cfg.Collection.Command, specs[0].Plugin)
	}
}

func TestBuildJobSpecsHTTPPlugin(t *testing.T) {
	cfg := JobConfig{
		Name: "http_job",
		Collection: CollectionConfig{
			Type: CollectionHTTP,
			HTTP: HTTPConfig{URL: "http://127.0.0.1/api"},
		},
	}
	cfg.Pipelines = []PipelineConfig{{
		Name:      "latency",
		Context:   "zabbix.http.latency",
		Dimension: "value",
		Unit:      "ms",
		Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
	}}
	specs, err := buildJobSpecs([]JobConfig{cfg})
	if err != nil {
		t.Fatalf("buildJobSpecs returned error: %v", err)
	}
	if specs[0].Plugin != "zabbix-http" {
		t.Fatalf("expected synthetic plugin 'zabbix-http', got %q", specs[0].Plugin)
	}
}

func TestBuildJobSpecsSNMPPlugin(t *testing.T) {
	cfg := JobConfig{
		Name: "snmp_job",
		Collection: CollectionConfig{
			Type: CollectionSNMP,
			SNMP: SNMPConfig{Target: "127.0.0.1", OID: "1.3.6.1.2.1.1.1.0"},
		},
	}
	cfg.Pipelines = []PipelineConfig{{
		Name:      "value",
		Context:   "zabbix.snmp.value",
		Dimension: "value",
		Unit:      "",
		Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
	}}
	specs, err := buildJobSpecs([]JobConfig{cfg})
	if err != nil {
		t.Fatalf("buildJobSpecs returned error: %v", err)
	}
	if specs[0].Plugin != "zabbix-snmp" {
		t.Fatalf("expected synthetic plugin 'zabbix-snmp', got %q", specs[0].Plugin)
	}
}

func TestBuildJobSpecsUpdateEvery(t *testing.T) {
	cfg := JobConfig{
		Name:        "interval_job",
		UpdateEvery: 42,
		Collection: CollectionConfig{
			Type:    CollectionCommand,
			Command: "/usr/bin/true",
		},
	}
	cfg.Pipelines = []PipelineConfig{{
		Name:      "value",
		Context:   "zabbix.interval.value",
		Dimension: "value",
		Unit:      "",
		Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
	}}
	specs, err := buildJobSpecs([]JobConfig{cfg})
	if err != nil {
		t.Fatalf("buildJobSpecs returned error: %v", err)
	}
	want := time.Duration(cfg.UpdateEvery) * time.Second
	if specs[0].CheckInterval != want {
		t.Fatalf("expected check interval %s, got %s", want, specs[0].CheckInterval)
	}
}

func TestBuildJobSpecsDefaultInterval(t *testing.T) {
	cfg := JobConfig{
		Name: "default_interval_job",
		Collection: CollectionConfig{
			Type:    CollectionCommand,
			Command: "/usr/bin/true",
			Timeout: confopt.Duration(5 * time.Second),
		},
	}
	cfg.Pipelines = []PipelineConfig{{
		Name:      "value",
		Context:   "zabbix.interval.value",
		Dimension: "value",
		Unit:      "",
		Steps:     []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.value"}},
	}}
	specs, err := buildJobSpecs([]JobConfig{cfg})
	if err != nil {
		t.Fatalf("buildJobSpecs returned error: %v", err)
	}
	if specs[0].CheckInterval != time.Minute {
		t.Fatalf("expected default check interval %s, got %s", time.Minute, specs[0].CheckInterval)
	}
}

func TestBuildCollectionMacros(t *testing.T) {
	cfg := JobConfig{Name: "disk", Collection: CollectionConfig{Type: CollectionCommand, Command: "/bin/true"}}
	job := runtime.JobRuntime{Spec: spec.JobSpec{Name: cfg.Name}, Vnode: runtime.VnodeInfo{Hostname: "agent", Address: "10.0.0.1", Alias: "agent-alias"}}
	macros := buildCollectionMacros(cfg, job)
	if macros["{HOST.NAME}"] != "agent" {
		t.Fatalf("expected host macro 'agent', got %q", macros["{HOST.NAME}"])
	}
	if macros["{HOST.IP}"] != "10.0.0.1" {
		t.Fatalf("expected host ip macro '10.0.0.1', got %q", macros["{HOST.IP}"])
	}
	if macros["{ITEM.KEY}"] == "" {
		t.Fatalf("expected item key macro to be populated")
	}
}

func TestRunHTTPMacroExpansion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("X-Host") != "agent" {
			t.Fatalf("expected header X-Host=agent, got %s", r.Header.Get("X-Host"))
		}
		if r.URL.Path != "/api/agent" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := JobConfig{
		Name: "http_job",
		Collection: CollectionConfig{
			Type: CollectionHTTP,
			HTTP: HTTPConfig{
				URL:     srv.URL + "/api/{HOST.NAME}",
				Headers: map[string]string{"X-Host": "{HOST.NAME}"},
				Body:    "value={ITEM.NAME}",
			},
			Method: "post",
		},
	}
	collector := newJobCollector([]JobConfig{cfg}, nil)
	job := runtime.JobRuntime{Spec: spec.JobSpec{Name: cfg.Name}, Vnode: runtime.VnodeInfo{Hostname: "agent"}}
	macros := buildCollectionMacros(cfg, job)
	data, _, _, err := collector.runHTTP(context.Background(), job, cfg, macros, time.Second)
	if err != nil {
		t.Fatalf("runHTTP returned error: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("expected response 'ok', got %q", string(data))
	}
}

func TestRequiresSNMPWalk(t *testing.T) {
	cfg := JobConfig{
		Name: "snmp_walk",
		Collection: CollectionConfig{
			Type: CollectionSNMP,
			SNMP: SNMPConfig{Target: "127.0.0.1", OID: ".1.3.6.1"},
		},
		Pipelines: []PipelineConfig{{
			Name:      "walk",
			Steps:     []zpre.Step{{Type: zpre.StepTypeSNMPWalkToJSON, Params: "{#IFNAME}\n.1.3.6.1\n1"}},
			Context:   "zabbix.snmp.walk",
			Dimension: "value",
			Unit:      "value",
		}},
	}
	if !requiresSNMPWalk(cfg) {
		t.Fatalf("expected requiresSNMPWalk to be true")
	}
}

func TestRunSNMPWalk(t *testing.T) {
	cfg := JobConfig{
		Name: "snmp_walk",
		Collection: CollectionConfig{
			Type: CollectionSNMP,
			SNMP: SNMPConfig{Target: "127.0.0.1", OID: ".1.3.6.1"},
		},
		Pipelines: []PipelineConfig{{
			Name:      "walk",
			Steps:     []zpre.Step{{Type: zpre.StepTypeSNMPWalkToJSON, Params: "{#IFNAME}\n.1.3.6.1\n1"}},
			Context:   "zabbix.snmp.walk",
			Dimension: "value",
			Unit:      "value",
		}},
	}
	if !requiresSNMPWalk(cfg) {
		t.Fatalf("expected requiresSNMPWalk to be true")
	}
}
