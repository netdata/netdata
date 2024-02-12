// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"

	"github.com/ilyam8/hashstructure"
)

type ConfigFileProvider interface {
	Run(ctx context.Context)
	Configs() chan ConfigFile
}

type ConfigFile struct {
	Source string
	Data   []byte
}

func (c *ConfigFile) Hash() uint64 {
	h, _ := hashstructure.Hash(c, nil)
	return h
}
