// SPDX-License-Identifier: GPL-3.0-or-later

package filepersister

import (
	"os"
)

func Save(path string, data interface{ Bytes() ([]byte, error) }) {
	if path == "" {
		return
	}
	bs, err := data.Bytes()
	if err != nil {
		return
	}
	_ = os.WriteFile(path, bs, 0644)
}
