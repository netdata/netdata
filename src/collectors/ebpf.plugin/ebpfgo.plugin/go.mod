module github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin

go 1.26.0

// Packages from go/plugins (netipc, netdataapi) are co-developed in this
// repository.  The replace directive lets the module resolve them from the
// local tree when go.work is not active (e.g. RPM/DEB package builds).
// go.work takes precedence over this directive in workspace-aware builds.
require github.com/netdata/netdata/go/plugins v0.0.0-00010101000000-000000000000

replace github.com/netdata/netdata/go/plugins => ../../../go
