<!--
title: "tlscfg"
custom_edit_url: "https://github.com/netdata/go.d.plugin/edit/master/pkg/tlscfg/README.md"
sidebar_label: "tlscfg"
learn_status: "Published"
learn_rel_path: "Developers/External plugins/go.d.plugin/Helper Packages"
-->

# tlscfg

This package contains client TLS configuration and function to create `tls.Config` from it.

Every module that needs `tls.Config` for data collection should use it. It allows to have same set of user configurable
options across all modules.

## Configuration options

- `tls_skip_verify`: controls whether a client verifies the server's certificate chain and host name.
- `tls_ca`: certificate authority to use when verifying server certificates.
- `tls_cert`: tls certificate to use.
- `tls_key`: tls key to use.

## Usage

Just make `TLSConfig` part of your module configuration.

```go
package example

import "github.com/netdata/go.d.plugin/pkg/tlscfg"

type Config struct {
	tlscfg.TLSConfig `yaml:",inline"`
}

type Example struct {
	Config `yaml:",inline"`
}

func (e *Example) Init() bool {
	tlsCfg, err := tlscfg.NewTLSConfig(e.TLSConfig)
	if err != nil {
		// ...
		return false
	}

	// ...
	return true
}
```

Having `TLSConfig` embedded your configuration inherits all [configuration options](#configuration-options):

```yaml
jobs:
  - name: name
    tls_skip_verify: no
    tls_ca: path/to/ca.pem
    tls_cert: path/to/cert.pem
    tls_key: path/to/key.pem
```
