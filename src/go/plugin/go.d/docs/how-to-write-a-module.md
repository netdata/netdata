# How to write a Netdata collector in Go

## Prerequisites

- Take a look at our [contributing guidelines](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md).
- [Fork](https://docs.github.com/en/github/getting-started-with-github/fork-a-repo) this repository to your personal
  GitHub account.
- [Clone](https://docs.github.com/en/github/creating-cloning-and-archiving-repositories/cloning-a-repository#:~:text=to%20GitHub%20Desktop-,On%20GitHub%2C%20navigate%20to%20the%20main%20page%20of%20the%20repository,Desktop%20to%20complete%20the%20clone.)
  locally the **forked** repository (e.g `git clone https://github.com/odyslam/go.d.plugin`).
- Using a terminal, `cd` into the directory (e.g `cd go.d.plugin`)

## Write and test a simple collector

> :exclamation: You can skip most of these steps if you first experiment directly with the existing
> [example module](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/example), which
> will
> give you an idea of how things work.

Let's assume you want to write a collector named `example2`.

The steps are:

- Add the source code
  to [`modules/example2/`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector).
  - [module interface](#module-interface).
  - [suggested module layout](#module-layout).
  - [helper packages](#helper-packages).
- Add the configuration
  to [`config/go.d/example2.conf`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/config/go.d).
- Add the module
  to [`config/go.d.conf`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/config/go.d.conf).
- Import the module
  in [`modules/init.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/init.go).
- Update
  the [`available modules list`](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d#available-modules).
- To build it, run `make` from the plugin root dir. This will create a new `go.d.plugin` binary that includes your newly
  developed collector. It will be placed into the `bin` directory (e.g `go.d.plugin/bin`)
- Run it in the debug mode `bin/godplugin -d -m <MODULE_NAME>`. This will output the `STDOUT` of the collector, the same
  output that is sent to the Netdata Agent and is transformed into charts. You can read more about this collector API in
  our [documentation](/src/plugins.d/README.md#external-plugins-api).
- If you want to test the collector with the actual Netdata Agent, you need to replace the `go.d.plugin` binary that
  exists in the Netdata Agent installation directory with the one you just compiled. Once
  you restart the Netdata Agent, it will detect and run it, creating all the charts. It is advised not to remove the default `go.d.plugin` binary, but simply rename it to `go.d.plugin.old` so that the Agent doesn't run it, but you can easily rename it back once you are done.
- Run `make clean` when you are done with testing.

## Module Interface

Every module should implement the following interface:

```go
type Module interface {
    Init() bool
    Check() bool
    Charts() *Charts
    Collect() map[string]int64
    Cleanup()
}
```

### Init method

- `Init` does module initialization.
- If it returns `false`, the job will be disabled.

We propose to use the following template:

```go
// example.go

func (e *Example) Init() bool {
    err := e.validateConfig()
    if err != nil {
        e.Errorf("config validation: %v", err)
        return false
    }

    someValue, err := e.initSomeValue()
    if err != nil {
        e.Errorf("someValue init: %v", err)
        return false
    }
    e.someValue = someValue

    // ...
    return true 
}
```

Move specific initialization methods into the `init.go` file. See [suggested module layout](#module-layout).

### Check method

- `Check` returns whether the job is able to collect metrics.
- Called after `Init` and only if `Init` returned `true`.
- If it returns `false`, the job will be disabled.

The simplest way to implement `Check` is to see if we are getting any metrics from `Collect`. A lot of modules use such
approach.

```go
// example.go

func (e *Example) Check() bool {
    return len(e.Collect()) > 0
}
```

### Charts method

:exclamation: Netdata module
produces [`charts`](/src/plugins.d/README.md#chart), not
raw metrics.

Use [`agent/module`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/agent/module/charts.go)
package to create them,
it contains charts and dimensions structs.

- `Charts` returns
  the [charts](/src/plugins.d/README.md#chart) (`*module.Charts`).
- Called after `Check` and only if `Check` returned `true`.
- If it returns `nil`, the job will be disabled
- :warning: Make sure not to share returned value between module instances (jobs).

Usually charts initialized in `Init` and `Chart` method just returns the charts instance:

```go
// example.go

func (e *Example) Charts() *Charts {
    return e.charts
}
```

### Collect method

- `Collect` collects metrics.
- Called only if `Check` returned `true`.
- Called every `update_every` seconds.
- `map[string]int64` keys are charts dimensions ids'.

We propose to use the following template:

```go
// example.go

func (e *Example) Collect() map[string]int64 {
    ms, err := e.collect()
    if err != nil {
        e.Error(err)
    }

    if len(ms) == 0 {
        return nil
    }
    return ms
}
```

Move metrics collection logic into the `collect.go` file. See [suggested module layout](#module-layout).

### Cleanup method

- `Cleanup` performs the job cleanup/teardown.
- Called if `Init` or `Check` fails, or we want to stop the job after `Collect`.

If you have nothing to clean up:

```go
// example.go

func (Example) Cleanup() {}
```

## Module Layout

The general idea is to not put everything in a single file.

We recommend using one file per logical area. This approach makes it easier to maintain the module.

Suggested minimal layout:

| Filename                                          | Contains                                               |
|---------------------------------------------------|--------------------------------------------------------|
| [`module_name.go`](#file-module_namego)           | Module configuration, implementation and registration. |
| [`charts.go`](#file-chartsgo)                     | Charts, charts templates and constructor functions.    |
| [`init.go`](#file-initgo)                         | Initialization methods.                                |
| [`collect.go`](#file-collectgo)                   | Metrics collection implementation.                     |
| [`module_name_test.go`](#file-module_name_testgo) | Public methods/functions tests.                        |
| [`testdata/`](#file-module_name_testgo)           | Files containing sample data.                          |

### File `module_name.go`

> :exclamation: See the
> example [`example.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/example/example.go).

Don't overload this file with the implementation details.

Usually it contains only:

- module registration.
- module configuration.
- [module interface implementation](#module-interface).

### File `charts.go`

> :exclamation: See the
> example: [`charts.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/example/charts.go).

Put charts, charts templates and charts constructor functions in this file.

### File `init.go`

> :exclamation: See the
> example: [`init.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/example/init.go).

All the module initialization details should go in this file.

- make a function for each value that needs to be initialized.
- a function should return a value(s), not implicitly set/change any values in the main struct.

```go
// init.go

// Prefer this approach.
func (e Example) initSomeValue() (someValue, error) {
    // ...
    return someValue, nil 
}

// This approach is ok too, but we recommend to not use it.
func (e *Example) initSomeValue() error {
    // ...
    m.someValue = someValue
    return nil
}
```

### File `collect.go`

> :exclamation: See the
> example: [`collect.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/example/collect.go).

This file is the entry point for the metrics collection.

Feel free to split it into several files if you think it makes the code more readable.

Use `collect_` prefix for the filenames: `collect_this.go`, `collect_that.go`, etc.

```go
// collect.go

func (e *Example) collect() (map[string]int64, error) {
    collected := make(map[string])int64
    // ...
    // ...
    // ...
    return collected, nil
}
```

### File `module_name_test.go`

> :exclamation: See the
> example: [`example_test.go`](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/example/example_test.go).
>
> if you have no experience in testing we recommend starting
> with [testing package documentation](https://golang.org/pkg/testing/).
>
> we use `assert` and `require` packages from [github.com/stretchr/testify](https://github.com/stretchr/testify)
> library,
> check [their documentation](https://pkg.go.dev/github.com/stretchr/testify).

Testing is mandatory.

- test only public functions and methods (`New`, `Init`, `Check`, `Charts`, `Cleanup`, `Collect`).
- do not create a test function per a case, use [table driven tests](https://github.com/golang/go/wiki/TableDrivenTests)
  . Prefer `map[string]struct{ ... }` over `[]struct{ ... }`.
- use helper functions _to prepare_ test cases to keep them clean and readable.

### Directory `testdata/`

Put files with sample data in this directory if you need any. Its name should
be [`testdata`](https://golang.org/cmd/go/#hdr-Package_lists_and_patterns).

> Directory and file names that begin with "." or "_" are ignored by the go tool, as are directories named "testdata".

## Helper packages

There are [some helper packages](https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/pkg) for
writing a module.
