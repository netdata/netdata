// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/logs"
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
	lastLine, err := logs.ReadLastLine(s.file.CurrentFilename(), 0)
	if err != nil {
		return fmt.Errorf("read last line: %v", err)
	}

	lastLine = bytes.TrimRight(lastLine, "\n")
	s.Debugf("last line: '%s'", string(lastLine))

	s.parser, err = logs.NewParser(s.Parser, s.file)
	if err != nil {
		return fmt.Errorf("create parser: %v", err)
	}
	s.Debugf("created parser: %s", s.parser.Info())

	err = s.parser.Parse(lastLine, s.line)
	if err != nil {
		return fmt.Errorf("parse last line: %v (%s)", err, string(lastLine))
	}

	if err = s.line.verify(); err != nil {
		return fmt.Errorf("verify last line: %v (%s)", err, string(lastLine))
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
