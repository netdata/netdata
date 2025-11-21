// SPDX-License-Identifier: GPL-3.0-or-later

package web

func ExampleHTTPConfig_usage() {
	// Just embed HTTPConfig into your module structure.
	// It allows you to have both RequestConfig and ClientConfig fields in the module configuration file.
	type myModule struct {
		HTTPConfig `yaml:",inline"`
	}

	var m myModule
	_, _ = NewHTTPRequest(m.RequestConfig)
	_, _ = NewHTTPClient(m.ClientConfig)
}
