// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
)

type deviceIdentity struct {
	name   string
	typ    string
	prefix string
}

func newDeviceIdentity(name, typ string) deviceIdentity {
	rawName, rawType := name, typ
	name, nameChanged := replaceIDWhitespace(name)
	typ, typeChanged := replaceIDWhitespace(typ)

	if nameChanged || typeChanged {
		digest := sha256.Sum256([]byte(rawName + "\x00" + rawType))
		name += fmt.Sprintf("_%x", digest[:8])
	}

	return deviceIdentity{
		name:   name,
		typ:    typ,
		prefix: fmt.Sprintf("device_%s_type_%s_", name, typ),
	}
}

func replaceIDWhitespace(value string) (string, bool) {
	changed := false
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			changed = true
			return '_'
		}
		return r
	}, value)
	return value, changed
}
