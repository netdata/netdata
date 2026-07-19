// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func TestFunctionAssemblyLifecycle(t *testing.T) {
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &assemblyTestHandler{}
			},
		},
	}
	assembly, err := NewFunctionAssembly(7, modules, frames)
	if err != nil {
		t.Fatal(err)
	}
	catalog, ok := assembly.Catalog().(*functionadapter.Catalog)
	if !ok {
		t.Fatal("assembly did not expose its exact Function catalog")
	}
	mutations := &assemblyMutationPort{catalog: catalog}
	if err := assembly.Bind(mutations); err != nil {
		t.Fatal(err)
	}
	if err := assembly.Activate(); err != nil {
		t.Fatal(err)
	}
	const registration = "FUNCTION GLOBAL \"module:method\" 60 \"module method data function\" \"top\" 0x0000 100 3\n\n"
	if got := output.String(); got != registration {
		t.Fatalf("activation frame=%q", got)
	}
	if err := assembly.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != registration+
		"FUNCTION_DEL GLOBAL \"module:method\"\n\n" {
		t.Fatalf("lifecycle frames=%q", got)
	}
}

func TestFunctionAssemblyStateGuards(t *testing.T) {
	tests := map[string]struct {
		run func(*FunctionAssembly, *assemblyMutationPort) error
	}{
		"activate before bind": {
			run: func(assembly *FunctionAssembly, _ *assemblyMutationPort) error {
				return assembly.Activate()
			},
		},
		"duplicate bind": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				return assembly.Bind(mutations)
			},
		},
		"duplicate activation": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				return assembly.Activate()
			},
		},
		"bind after stop": {
			run: func(assembly *FunctionAssembly, mutations *assemblyMutationPort) error {
				if err := assembly.Bind(mutations); err != nil {
					return err
				}
				if err := assembly.Activate(); err != nil {
					return err
				}
				if err := assembly.Stop(); err != nil {
					return err
				}
				return assembly.Bind(mutations)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			assembly, err := NewFunctionAssembly(1, collectorapi.Registry{}, frames)
			if err != nil {
				t.Fatal(err)
			}
			catalog := assembly.Catalog().(*functionadapter.Catalog)
			if err := test.run(assembly, &assemblyMutationPort{catalog: catalog}); err == nil {
				t.Fatal("invalid assembly transition was accepted")
			}
		})
	}
}

func TestFunctionProtocolPanicPreservesTaskClassification(t *testing.T) {
	err := callFunctionProtocol(func() {
		panic("handler failed")
	})
	if !errors.Is(err, lifecycle.ErrTaskPanic) {
		t.Fatalf("panic error=%v want ErrTaskPanic", err)
	}
}

func TestFunctionAssemblyJobHookCapturesExactHandle(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := NewFunctionAssembly(
		1,
		collectorapi.Registry{"module": {}},
		frames,
	)
	if err != nil {
		t.Fatal(err)
	}
	job := &assemblyTestJob{}
	handle, err := assembly.JobHooks().Prepare(joboutput.PublishedJob{
		Identity: lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 3},
		Variant:  joboutput.JobVariantV1,
		Job:      job,
	})
	if err != nil {
		t.Fatal(err)
	}
	if handle == nil {
		t.Fatal("job hook returned no lifecycle handle")
	}
	if err := handle.CloseAndDrain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := handle.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type assemblyMutationPort struct {
	catalog *functionadapter.Catalog
}

func (port *assemblyMutationPort) QuiesceFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if port == nil || port.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := port.catalog.BeginMutation(mutation); err != nil {
		return err
	}
	for {
		progress, err := port.catalog.AdvanceMutationQuiesce(
			jobmgr.MaximumFunctionMutationQuantum,
		)
		if err != nil {
			return err
		}
		if progress.Quiesced {
			return nil
		}
	}
}

func (port *assemblyMutationPort) CommitFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) (uint64, error) {
	if port == nil || port.catalog == nil {
		return 0, errors.New("nil mutation port")
	}
	if err := port.catalog.ResumeMutation(mutation); err != nil {
		return 0, err
	}
	for {
		var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
		progress, count, err := port.catalog.AdvanceMutation(
			jobmgr.MaximumFunctionMutationQuantum,
			&cleanups,
		)
		if err != nil {
			return 0, err
		}
		for index := 0; index < count; index++ {
			cleanup := cleanups[index]
			_, cleanupErr := cleanup.Runner.RunTask(context.Background())
			if err := port.catalog.CompleteCleanup(cleanup.Ref, cleanupErr); err != nil {
				return 0, errors.Join(cleanupErr, err)
			}
		}
		if progress.Done {
			return progress.Version, nil
		}
	}
}

func (port *assemblyMutationPort) AbortFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if port == nil || port.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := port.catalog.ResumeMutation(mutation); err != nil {
		return err
	}
	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, err := port.catalog.AbortMutation(&cleanups)
	if err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		cleanup := cleanups[index]
		_, cleanupErr := cleanup.Runner.RunTask(context.Background())
		if err := port.catalog.CompleteCleanup(
			cleanup.Ref,
			cleanupErr,
		); err != nil {
			return errors.Join(cleanupErr, err)
		}
	}
	return nil
}

type assemblyTestHandler struct{}

func (*assemblyTestHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*assemblyTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (*assemblyTestHandler) Cleanup(context.Context) {}

type assemblyTestJob struct{}

func (*assemblyTestJob) FullName() string   { return "module_job" }
func (*assemblyTestJob) ModuleName() string { return "module" }
func (*assemblyTestJob) Name() string       { return "job" }
func (*assemblyTestJob) IsRunning() bool    { return true }
func (*assemblyTestJob) Collector() any     { return nil }
func (*assemblyTestJob) AutoDetection(context.Context) error {
	return nil
}
func (*assemblyTestJob) AutoDetectionManaged(context.Context) error { return nil }
func (*assemblyTestJob) CleanupRejected()                           {}
func (*assemblyTestJob) Tick(int)                                   {}
func (*assemblyTestJob) Cleanup()                                   {}
func (*assemblyTestJob) Stop()                                      {}
func (*assemblyTestJob) StartManaged(ready chan<- struct{}) {
	close(ready)
}
