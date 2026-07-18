package agentdriver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/driverkit"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/perffixture"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const wireAgentMode = "wire/agent"

type options struct {
	mode              string
	fixtureConfigDir  string
	eventFD           int
	processGeneration uint64
}

func Main(args []string) int {
	if err := run(args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	eventFile := os.NewFile(uintptr(opts.eventFD), "jobmgrtest-events")
	if eventFile == nil {
		return errors.New("jobmgr test agent: event file descriptor is unavailable")
	}
	defer eventFile.Close()
	emitter, err := driverkit.NewEmitter(eventFile, opts.processGeneration)
	if err != nil {
		return err
	}
	switch opts.mode {
	case wireAgentMode:
		return runWireAgent(opts.fixtureConfigDir, emitter)
	default:
		return fmt.Errorf("jobmgr test agent: unsupported mode %q", opts.mode)
	}
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("jobmgrtest-agent", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.mode, "mode", "", "public driver mode")
	flags.StringVar(&opts.fixtureConfigDir, "fixture-config-dir", "", "immutable fixture config directory")
	flags.IntVar(&opts.eventFD, "event-fd", -1, "passive event file descriptor")
	flags.Uint64Var(&opts.processGeneration, "process-generation", 0, "process generation")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("jobmgr test agent: positional arguments are forbidden")
	}
	if opts.mode != wireAgentMode {
		return options{}, fmt.Errorf("jobmgr test agent: unsupported mode %q", opts.mode)
	}
	if !filepath.IsAbs(opts.fixtureConfigDir) {
		return options{}, errors.New("jobmgr test agent: fixture config directory must be absolute")
	}
	info, err := os.Stat(filepath.Join(opts.fixtureConfigDir, perffixture.ModuleName+".conf"))
	if err != nil || !info.Mode().IsRegular() {
		return options{}, errors.New("jobmgr test agent: immutable fixture config is unavailable")
	}
	if opts.eventFD < 3 || opts.eventFD > 1024 {
		return options{}, errors.New("jobmgr test agent: event file descriptor must be within 3..1024")
	}
	if opts.processGeneration == 0 {
		return options{}, errors.New("jobmgr test agent: process generation must be positive")
	}
	return opts, nil
}

func runWireAgent(configDir string, emitter *driverkit.Emitter) error {
	logger.Level.SetByName("critical")
	instance := agent.New(agent.Config{
		Name:                    "poc",
		PluginConfigDir:         []string{configDir},
		CollectorsConfigDir:     []string{configDir},
		VarLibDir:               configDir,
		ModuleRegistry:          collectorapi.Registry{perffixture.ModuleName: performanceCreator(emitter)},
		RunModule:               perffixture.ModuleName,
		MinUpdateEvery:          1,
		DisableServiceDiscovery: true,
		RunModePolicy:           policy.Agent(true),
		DiscoveryProviders:      []discovery.ProviderFactory{fileProvider(), dummyProvider()},
	})
	return instance.RunContext(context.Background())
}

func fileProvider() discovery.ProviderFactory {
	return discovery.NewProviderFactory("file", func(build discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(build.ReadPaths)+len(build.Paths.CollectorsConfigWatchPath) == 0 {
			return nil, false, nil
		}
		provider, err := file.NewDiscovery(file.Config{
			Registry: build.Registry, Read: build.ReadPaths, Watch: build.Paths.CollectorsConfigWatchPath,
		})
		return provider, err == nil, err
	})
}

func dummyProvider() discovery.ProviderFactory {
	return discovery.NewProviderFactory("dummy", func(build discovery.BuildContext) (discovery.Discoverer, bool, error) {
		if len(build.DummyNames) == 0 {
			return nil, false, nil
		}
		provider, err := dummy.NewDiscovery(dummy.Config{Registry: build.Registry, Names: build.DummyNames})
		return provider, err == nil, err
	})
}
