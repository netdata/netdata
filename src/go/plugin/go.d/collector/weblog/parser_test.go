// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebLog_guessParser(t *testing.T) {
	type test = struct {
		name           string
		inputs         []string
		wantParserType string
		wantErr        bool
	}
	tests := []test{
		{
			name:           "guessed csv",
			wantParserType: logs.TypeCSV,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123 0.123,0.321`,
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123`,
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123 0.123,0.321`,
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123`,
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123 0.123,0.321`,
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123`,
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123 0.123,0.321`,
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123`,
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
			},
		},
		{
			name:           "guessed ltsv",
			wantParserType: logs.TypeLTSV,
			inputs: []string{
				`field1:test.example.com:80 field2:88.191.254.20 field3:"GET / HTTP/1.0" 200 8674 field4:8674 field5:0.123`,
			},
		},
		{
			name:           "guessed json",
			wantParserType: logs.TypeJSON,
			inputs: []string{
				`{}`,
				` {}`,
				` {} `,
				`{"host": "example.com"}`,
				`{"host": "example.com","time": "2020-08-04T20:23:27+03:00", "upstream_response_time": "0.776", "remote_addr": "1.2.3.4"}`,
				` {"host": "example.com","time": "2020-08-04T20:23:27+03:00", "upstream_response_time": "0.776", "remote_addr": "1.2.3.4"}	`,
			},
		},
		{
			name:    "unknown",
			wantErr: true,
			inputs: []string{
				`test.example.com 80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
				`test.example.com 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
			},
		},
	}

	weblog := prepareWebLog()

	for _, tc := range tests {
		for i, input := range tc.inputs {
			name := fmt.Sprintf("name=%s,input_num=%d", tc.name, i+1)

			t.Run(name, func(t *testing.T) {
				p, err := weblog.newParser([]byte(input))

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					switch tc.wantParserType {
					default:
						t.Errorf("unknown parser type: %s", tc.wantParserType)
					case logs.TypeLTSV:
						assert.IsType(t, (*logs.LTSVParser)(nil), p)
					case logs.TypeCSV:
						require.IsType(t, (*logs.CSVParser)(nil), p)
					case logs.TypeJSON:
						require.IsType(t, (*logs.JSONParser)(nil), p)
					}
				}
			})
		}
	}
}

func TestWebLog_guessCSVParser(t *testing.T) {
	type test = struct {
		name          string
		inputs        []string
		wantCSVFormat string
		wantErr       bool
	}
	tests := []test{
		{
			name:          "guessed vhost custom4",
			wantCSVFormat: csvVhostCustom4,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123 0.123,0.321`,
			},
		},
		{
			name:          "guessed vhost custom3",
			wantCSVFormat: csvVhostCustom3,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123`,
			},
		},
		{
			name:          "guessed vhost custom2",
			wantCSVFormat: csvVhostCustom2,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123 0.123,0.321`,
			},
		},
		{
			name:          "guessed vhost custom1",
			wantCSVFormat: csvVhostCustom1,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123`,
			},
		},
		{
			name:          "guessed vhost common",
			wantCSVFormat: csvVhostCommon,
			inputs: []string{
				`test.example.com:80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
			},
		},
		{
			name:          "guessed custom4",
			wantCSVFormat: csvCustom4,
			inputs: []string{
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123 0.123,0.321`,
			},
		},
		{
			name:          "guessed custom3",
			wantCSVFormat: csvCustom3,
			inputs: []string{
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 "-" "-" 8674 0.123`,
			},
		},
		{
			name:          "guessed custom2",
			wantCSVFormat: csvCustom2,
			inputs: []string{
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123 0.123,0.321`,
			},
		},
		{
			name:          "guessed custom1",
			wantCSVFormat: csvCustom1,
			inputs: []string{
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674 8674 0.123`,
			},
		},
		{
			name:          "guessed common",
			wantCSVFormat: csvCommon,
			inputs: []string{
				`88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
			},
		},
		{
			name:    "unknown",
			wantErr: true,
			inputs: []string{
				`test.example.com 80 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
				`test.example.com 88.191.254.20 - - [22/Mar/2009:09:30:31 +0100] "GET / HTTP/1.0" 200 8674`,
			},
		},
	}

	weblog := prepareWebLog()

	for i, tc := range tests {
		for _, input := range tc.inputs {
			name := fmt.Sprintf("name=%s,input_num=%d", tc.name, i+1)

			t.Run(name, func(t *testing.T) {
				p, err := weblog.guessCSVParser([]byte(input))

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, cleanCSVFormat(tc.wantCSVFormat), p.(*logs.CSVParser).Config.Format)
				}
			})
		}
	}
}

func prepareWebLog() *Collector {
	cfg := logs.ParserConfig{
		LogType: typeAuto,
		CSV: logs.CSVConfig{
			Delimiter: " ",
		},
		LTSV: logs.LTSVConfig{
			FieldDelimiter: "\t",
			ValueDelimiter: ":",
		},
	}
	cfg.CSV.SetCheckField(checkCSVFormatField)

	return &Collector{
		Config: Config{
			GroupRespCodes: false,
			ParserConfig:   cfg,
		},
	}
}
