// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"github.com/clbanning/rfile/v2"
)

func ReadLastLines(filename string, n uint) ([]string, error) {
	return rfile.Tail(filename, int(n))
}
