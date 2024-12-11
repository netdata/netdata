// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

func (c *Collector) createLogReader() error {
	c.Cleanup(context.Background())
	c.Debug("starting log reader creating")

	reader, err := logs.Open(c.Path, c.ExcludePath, c.Logger)
	if err != nil {
		return fmt.Errorf("creating log reader: %v", err)
	}

	c.Debugf("created log reader, current file '%s'", reader.CurrentFilename())
	c.file = reader
	return nil
}

func (c *Collector) createParser() error {
	c.Debug("starting parser creating")

	const readLastLinesNum = 100

	lines, err := logs.ReadLastLines(c.file.CurrentFilename(), readLastLinesNum)
	if err != nil {
		return fmt.Errorf("failed to read last lines: %v", err)
	}

	var found bool
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}

		c.Debugf("last line: '%s'", line)

		c.parser, err = logs.NewParser(c.ParserConfig, c.file)
		if err != nil {
			c.Debugf("failed to create parser from line: %v", err)
			continue
		}

		c.line.reset()

		if err = c.parser.Parse([]byte(line), c.line); err != nil {
			c.Debugf("failed to parse line: %v", err)
			continue
		}

		if err = c.line.verify(); err != nil {
			c.Debugf("failed to verify line: %v", err)
			continue
		}

		found = true
		break
	}

	if !found {
		return fmt.Errorf("failed to create log parser (file '%s')", c.file.CurrentFilename())
	}

	return nil
}

func checkCSVFormatField(name string) (newName string, offset int, valid bool) {
	name = cleanField(name)
	if !knownField(name) {
		return "", 0, false
	}
	return name, 0, true
}

func cleanField(name string) string {
	return strings.TrimLeft(name, "$%")
}

func knownField(name string) bool {
	switch name {
	case fieldRespTime, fieldClientAddr, fieldCacheCode, fieldHTTPCode, fieldRespSize, fieldReqMethod:
		fallthrough
	case fieldHierCode, fieldServerAddr, fieldMimeType, fieldResultCode, fieldHierarchy:
		return true
	}
	return false
}
