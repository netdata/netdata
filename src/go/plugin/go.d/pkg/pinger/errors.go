// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import "fmt"

type ProbeError struct {
	Host  string
	Stage string
	Err   error
}

func (e *ProbeError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("ping %s for host %q", e.Stage, e.Host)
	}
	return fmt.Sprintf("ping %s for host %q: %v", e.Stage, e.Host, e.Err)
}

func (e *ProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
