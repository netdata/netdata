// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
)

func ExampleHTTPConfig_usage() {
	// Just embed HTTPConfig into your module structure.
	// It allows you to have both RequestConfig and ClientConfig fields in the module configuration file.
	type myModule struct {
		HTTPConfig `yaml:",inline"`
	}

	var m myModule
	client, err := NewHTTPClient(context.Background(), m.ClientConfig)
	if err != nil {
		return
	}
	defer client.CloseIdleConnections()
	_, _ = NewHTTPRequest(context.Background(), m.RequestConfig)
}
