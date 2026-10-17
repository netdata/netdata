module github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin

go 1.27.0

require (
	// Packages from go/plugins (netipc, netdataapi) are co-developed in this
	// repository.  The replace directive lets the module resolve them from the
	// local tree when go.work is not active (e.g. RPM/DEB package builds).
	// go.work takes precedence over this directive in workspace-aware builds.
	github.com/netdata/netdata/go/plugins v0.0.0-00010101000000-000000000000
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/net v0.59.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.23.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.14.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.8.0 // indirect
	github.com/axiomhq/hyperloglog v0.2.6 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/dgryski/go-metro v0.0.0-20250106013310-edb8663e5e33 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gohugoio/hashstructure v1.1.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gosnmp/gosnmp v1.42.1 // indirect
	github.com/jessevdk/go-flags v1.6.1 // indirect
	github.com/kamstrup/intmap v0.5.2 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lmittmann/tint v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/netdata/netdata/go/plugins => ../../../go
