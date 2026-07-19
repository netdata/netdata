// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"fmt"
	"time"
)

// Function is one parsed Function-protocol request.
type Function struct {
	key         string
	UID         string
	Timeout     time.Duration
	Name        string
	Args        []string
	Payload     []byte
	Permissions string
	Source      string
	ContentType string
}

func (f *Function) String() string {
	return fmt.Sprintf("key: '%s', uid: '%s', timeout: '%s', function: '%s', args: '%v', permissions: '%s', source: '%s',  contentType: '%s', payload: '%s'",
		f.key, f.UID, f.Timeout, f.Name, f.Args, f.Permissions, f.Source, f.ContentType, string(f.Payload))
}
