// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"time"
)

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
	Context     context.Context
}
