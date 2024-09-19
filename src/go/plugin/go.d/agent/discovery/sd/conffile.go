// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"os"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

type confFile struct {
	source  string
	content []byte
}

func newConfFileReader(log *logger.Logger, dir multipath.MultiPath) *confFileReader {
	return &confFileReader{
		Logger:   log,
		confDir:  dir,
		confChan: make(chan confFile),
	}
}

type confFileReader struct {
	*logger.Logger

	confDir  multipath.MultiPath
	confChan chan confFile
}

func (c *confFileReader) run(ctx context.Context) {
	files, err := c.confDir.FindFiles(".conf")
	if err != nil {
		c.Error(err)
		return
	}

	if len(files) == 0 {
		return
	}

	var confFiles []confFile

	for _, file := range files {
		bs, err := os.ReadFile(file)
		if err != nil {
			c.Error(err)
			continue
		}
		confFiles = append(confFiles, confFile{
			source:  file,
			content: bs,
		})
	}

	for _, conf := range confFiles {
		select {
		case <-ctx.Done():
		case c.confChan <- conf:
		}
	}

}

func (c *confFileReader) configs() chan confFile {
	return c.confChan
}
