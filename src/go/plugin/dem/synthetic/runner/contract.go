// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	_ "embed"
	"encoding/json"
)

// The same manifest supplies the bundle builder and the compiled runner checks.
// Runtime paths cannot change the versions accepted by this Agent build.
//
//go:embed assets/manifest.json
var runtimeManifest []byte

type runtimeContract struct {
	NodeMajorMin int               `json:"node_major_min"`
	Packages     map[string]string `json:"packages"`
	Chromium     struct {
		Version string `json:"version"`
	} `json:"chromium"`
}

var requiredRuntime = func() runtimeContract {
	var contract runtimeContract
	if err := json.Unmarshal(runtimeManifest, &contract); err != nil {
		panic("invalid embedded DEM runtime manifest: " + err.Error())
	}
	return contract
}()
