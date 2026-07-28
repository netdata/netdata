// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "time"

// Function is one parsed Function-protocol request.
type Function struct {
	UID         string
	Timeout     time.Duration
	Name        string
	Args        []string
	Payload     []byte
	Permissions string
	Source      string
	ContentType string
}
