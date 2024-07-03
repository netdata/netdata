// SPDX-License-Identifier: GPL-3.0-or-later

package web

func ExampleHTTP_usage() {
	// Just embed HTTP into your module structure.
	// It allows you to have both Request and Client fields in the module configuration file.
	type myModule struct {
		HTTP `yaml:",inline"`
	}

	var m myModule
	_, _ = NewHTTPRequest(m.Request)
	_, _ = NewHTTPClient(m.Client)
}
