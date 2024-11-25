// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

func (s *SquidLog) createLogReader() error {
	s.Cleanup()
	s.Debug("starting log reader creating")

	reader, err := logs.Open(s.Path, s.ExcludePath, s.Logger)
	if err != nil {
		return fmt.Errorf("creating log reader: %v", err)
	}

	s.Debugf("created log reader, current file '%s'", reader.CurrentFilename())
	s.file = reader
	return nil
}

func (s *SquidLog) createParser() error {
	s.Debug("starting parser creating")

	const readLastLinesNum = 100

	lines, err := logs.ReadLastLines(s.file.CurrentFilename(), readLastLinesNum)
	if err != nil {
		return fmt.Errorf("failed to read last lines: %v", err)
	}

	var found bool
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}

		s.Debugf("last line: '%s'", line)

		s.parser, err = logs.NewParser(s.ParserConfig, s.file)
		if err != nil {
			s.Debugf("failed to create parser from line: %v", err)
			continue
		}

		s.line.reset()

		if err = s.parser.Parse([]byte(line), s.line); err != nil {
			s.Debugf("failed to parse line: %v", err)
			continue
		}

		if err = s.line.verify(); err != nil {
			s.Debugf("failed to verify line: %v", err)
			continue
		}

		found = true
		break
	}

	if !found {
		return fmt.Errorf("failed to create log parser (file '%s')", s.file.CurrentFilename())
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
