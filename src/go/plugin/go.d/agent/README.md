# agent

This library is a tool for writing [netdata](https://github.com/netdata/netdata) plugins.

We strongly believe that custom plugins are very important, and they must be easy to write.


Definitions:
 - orchestrator
 > plugin orchestrators are external plugins that do not collect any data by themselves. Instead, they support data collection modules written in the language of the orchestrator. Usually the orchestrator provides a higher level abstraction, making it ideal for writing new data collection modules with the minimum of code.

 - plugin
 > plugin is a set of data collection modules.

 - module
 > module is a data collector. It collects, processes and returns processed data to the orchestrator.

 - job
 > job is a module instance with specific settings.


Package provides:
 - CLI parser
 - plugin orchestrator (loads configurations, creates and serves jobs)

You are responsible only for __creating modules__.

## Custom plugin example

[Yep! So easy!](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/examples/simple/main.go)

## How to write a Module

Module is responsible for **charts creating** and **data collecting**. Implement Module interface and that is it.

```go
type Module interface {
	// Init does initialization.
	// If it returns false, the job will be disabled.
	Init() bool

	// Check is called after Init.
	// If it returns false, the job will be disabled.
	Check() bool

	// Charts returns the chart definition.
	// Make sure not to share returned instance.
	Charts() *Charts

	// Collect collects metrics.
	Collect() map[string]int64

	// SetLogger sets logger.
	SetLogger(l *logger.Logger)

	// Cleanup performs cleanup if needed.
	Cleanup()
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

// SetLogger sets logger.
func (b *Base) SetLogger(l *logger.Logger) { b.Logger = l }

```

## How to write a Plugin

Since plugin is a set of modules all you need is:
 - write module(s)
 - add module(s) to the plugins [registry](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/plugin/module/registry.go)
 - start the plugin


## How to integrate your plugin into Netdata

Three simple steps:
 - move the plugin to the `plugins.d` dir.
 - add plugin configuration file to the `etc/netdata/` dir.
 - add modules configuration files to the `etc/netdata/<DIR_NAME>/` dir.

Congratulations!

## Configurations

Configurations are written in [YAML](https://yaml.org/).

 - plugin configuration:

```yaml

# Enable/disable the whole plugin.
enabled: yes

# Default enable/disable value for all modules.
default_run: yes

# Maximum number of used CPUs. Zero means no limit.
max_procs: 0

# Enable/disable specific plugin module
modules:
#  module_name1: yes
#  module_name2: yes

```

 - module configuration

```yaml
# [ GLOBAL ]
update_every: 1
autodetection_retry: 0

# [ JOBS ]
jobs:
  - name: job1
    param1: value1
    param2: value2

  - name: job2
    param1: value1
    param2: value2
```

Plugin uses `yaml.Unmarshal` to add configuration parameters to the module. Please use `yaml` tags!

## Debug

Plugin CLI:
```
Usage:
  plugin [OPTIONS] [update every]

Application Options:
  -d, --debug    debug mode
  -m, --modules= modules name (default: all)
  -c, --config=  config dir

Help Options:
  -h, --help     Show this help message

```

Specific module debug:
```
# become user netdata
sudo su -s /bin/bash netdata

# run plugin in debug mode
./<plugin_name> -d -m <module_name>
```

Change `<plugin_name>` to your plugin name and `<module_name>` to the module name you want to debug.
