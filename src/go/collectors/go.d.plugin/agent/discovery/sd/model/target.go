// SPDX-License-Identifier: GPL-3.0-or-later

package model

type Target interface {
	Hash() uint64
	Tags() Tags
	TUID() string
}

type TargetGroup interface {
	Targets() []Target
	Provider() string
	Source() string
}
