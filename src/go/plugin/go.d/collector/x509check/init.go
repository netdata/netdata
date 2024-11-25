// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"errors"
)

func (x *X509Check) validateConfig() error {
	if x.Source == "" {
		return errors.New("source is not set")
	}
	return nil
}

func (x *X509Check) initProvider() (provider, error) {
	return newProvider(x.Config)
}
