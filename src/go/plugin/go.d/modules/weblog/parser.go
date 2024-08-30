// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
)

/*
Default apache log format:
 - "%v:%p %h %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"" vhost_combined
 - "%h %l %u %t \"%r\" %>s %O \"%{Referer}i\" \"%{User-Agent}i\"" combined
 - "%h %l %u %t \"%r\" %>s %O" common
 - "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\" %I %O" Combined I/O (https://httpd.apache.org/docs/2.4/mod/mod_logio.html)

Default nginx log format:
 - '$remote_addr - $remote_user [$time_local] '
   '"$request" $status $body_bytes_sent '
   '"$http_referer" "$http_user_agent"' combined

Netdata recommends:
 Nginx:
  - '$remote_addr - $remote_user [$time_local] '
    '"$request" $status $body_bytes_sent '
    '$request_length $request_time $upstream_response_time '
    '"$http_referer" "$http_user_agent"'

 Apache:
  - "%h %l %u %t \"%r\" %>s %B %I %D \"%{Referer}i\" \"%{User-Agent}i\""
*/

var (
	csvCommon       = `                   $remote_addr - - [$time_local] "$request" $status $body_bytes_sent`
	csvCustom1      = `                   $remote_addr - - [$time_local] "$request" $status $body_bytes_sent     $request_length $request_time`
	csvCustom2      = `                   $remote_addr - - [$time_local] "$request" $status $body_bytes_sent     $request_length $request_time $upstream_response_time`
	csvCustom3      = `                   $remote_addr - - [$time_local] "$request" $status $body_bytes_sent - - $request_length $request_time`
	csvCustom4      = `                   $remote_addr - - [$time_local] "$request" $status $body_bytes_sent - - $request_length $request_time $upstream_response_time`
	csvVhostCommon  = `$host:$server_port $remote_addr - - [$time_local] "$request" $status $body_bytes_sent`
	csvVhostCustom1 = `$host:$server_port $remote_addr - - [$time_local] "$request" $status $body_bytes_sent     $request_length $request_time`
	csvVhostCustom2 = `$host:$server_port $remote_addr - - [$time_local] "$request" $status $body_bytes_sent     $request_length $request_time $upstream_response_time`
	csvVhostCustom3 = `$host:$server_port $remote_addr - - [$time_local] "$request" $status $body_bytes_sent - - $request_length $request_time`
	csvVhostCustom4 = `$host:$server_port $remote_addr - - [$time_local] "$request" $status $body_bytes_sent - - $request_length $request_time $upstream_response_time`

	guessOrder = []string{
		csvVhostCustom4,
		csvVhostCustom3,
		csvVhostCustom2,
		csvVhostCustom1,
		csvVhostCommon,
		csvCustom4,
		csvCustom3,
		csvCustom2,
		csvCustom1,
		csvCommon,
	}
)

func cleanCSVFormat(format string) string       { return strings.Join(strings.Fields(format), " ") }
func cleanApacheLogFormat(format string) string { return strings.ReplaceAll(format, `\`, "") }

const (
	typeAuto = "auto"
)

var (
	reLTSV = regexp.MustCompile(`^[a-zA-Z0-9]+:[^\t]*(\t[a-zA-Z0-9]+:[^\t]*)*$`)
	reJSON = regexp.MustCompile(`^[[:space:]]*{.*}[[:space:]]*$`)
)

func (w *WebLog) newParser(record []byte) (logs.Parser, error) {
	if w.ParserConfig.LogType == typeAuto {
		w.Debugf("log_type is %s, will try format auto-detection", typeAuto)
		if len(record) == 0 {
			return nil, fmt.Errorf("empty line, can't auto-detect format (%s)", w.file.CurrentFilename())
		}
		return w.guessParser(record)
	}

	w.ParserConfig.CSV.Format = cleanApacheLogFormat(w.ParserConfig.CSV.Format)
	w.Debugf("log_type is %s, skipping auto-detection", w.ParserConfig.LogType)
	switch w.ParserConfig.LogType {
	case logs.TypeCSV:
		w.Debugf("config: %+v", w.ParserConfig.CSV)
	case logs.TypeLTSV:
		w.Debugf("config: %+v", w.ParserConfig.LogType)
	case logs.TypeRegExp:
		w.Debugf("config: %+v", w.ParserConfig.RegExp)
	case logs.TypeJSON:
		w.Debugf("config: %+v", w.ParserConfig.JSON)
	}
	return logs.NewParser(w.ParserConfig, w.file)
}

func (w *WebLog) guessParser(record []byte) (logs.Parser, error) {
	w.Debug("starting log type auto-detection")
	if reLTSV.Match(record) {
		w.Debug("log type is LTSV")
		return logs.NewLTSVParser(w.ParserConfig.LTSV, w.file)
	}
	if reJSON.Match(record) {
		w.Debug("log type is JSON")
		return logs.NewJSONParser(w.ParserConfig.JSON, w.file)
	}
	w.Debug("log type is CSV")
	return w.guessCSVParser(record)
}

func (w *WebLog) guessCSVParser(record []byte) (logs.Parser, error) {
	w.Debug("starting csv log format auto-detection")
	w.Debugf("config: %+v", w.ParserConfig.CSV)
	for _, format := range guessOrder {
		format = cleanCSVFormat(format)
		cfg := w.ParserConfig.CSV
		cfg.Format = format

		w.Debugf("trying format: '%s'", format)
		parser, err := logs.NewCSVParser(cfg, w.file)
		if err != nil {
			return nil, err
		}

		line := newEmptyLogLine()
		if err := parser.Parse(record, line); err != nil {
			w.Debug("parse: ", err)
			continue
		}

		if err = line.verify(); err != nil {
			w.Debug("verify: ", err)
			continue
		}
		return parser, nil
	}
	return nil, errors.New("cannot auto-detect log format, use custom log format")
}

func checkCSVFormatField(field string) (newName string, offset int, valid bool) {
	if isTimeField(field) {
		return "", 1, false
	}
	if !isFieldValid(field) {
		return "", 0, false
	}
	// remove `$` and `%` to have same field names with regexp parser,
	// these symbols aren't allowed in sub exp names
	return field[1:], 0, true
}

func isTimeField(field string) bool {
	return field == "[$time_local]" || field == "$time_local" || field == "%t"
}

func isFieldValid(field string) bool {
	return len(field) > 1 && (isNginxField(field) || isApacheField(field))
}
func isNginxField(field string) bool {
	return strings.HasPrefix(field, "$")
}

func isApacheField(field string) bool {
	return strings.HasPrefix(field, "%")
}
