// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
)

func ExampleHTTPConfig_usage() {
	// Just embed HTTPConfig into your module structure.
	// It allows you to have both RequestConfig and ClientConfig fields in the module configuration file.
	type myModule struct {
		HTTPConfig `yaml:",inline"`
	}

	var m myModule
	files := credentialfile.New()
	defer files.Close()
	_, _ = NewHTTPRequest(context.Background(), m.RequestConfig, files)
	_, _ = NewHTTPClient(context.Background(), m.ClientConfig, files)
}
