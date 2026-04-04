// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "os"

func mustReadTestData(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic("failed to read " + path + ": " + err.Error())
	}
	return data
}
