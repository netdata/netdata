// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

// Unverified IDs are never promoted retroactively by a later successful read.
type bootIdentity struct {
	ID       string `json:"id,omitempty"`
	Verified bool   `json:"verified"`
}

func (b bootIdentity) supersedes(old bootIdentity) bool {
	return b.Verified && old.Verified && b.ID != old.ID
}

func validBootID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
		} else if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return value != "00000000-0000-0000-0000-000000000000"
}
