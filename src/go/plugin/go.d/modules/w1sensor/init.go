// // SPDX-License-Identifier: GPL-3.0-or-later

package w1sensor

import (
	"errors"
)

func (w *W1sensor) validateConfig() error {
	if w.SensorsPath == "" {
		return errors.New("no sensors path specified")
	}
	return nil
}
