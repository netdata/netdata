// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCSVConfig = CSVConfig{
	Delimiter: " ",
	Format:    "$A %B",
}

func TestNewCSVParser(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "valid format", format: "$A $B"},
		{name: "empty format", wantErr: true},
		{name: "bad format: csv read error", format: "$A $B \"$C", wantErr: true},
		{name: "bad format: duplicate fields", format: "$A $A", wantErr: true},
		{name: "bad format: zero fields", format: "!A !B", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testCSVConfig
			c.Format = tt.format
			p, err := NewCSVParser(c, nil)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

func TestNewCSVFormat(t *testing.T) {
	tests := []struct {
		format     string
		wantFormat csvFormat
		wantErr    bool
	}{
		{format: "$A $B", wantFormat: csvFormat{maxIndex: 1, fields: []csvField{{"$A", 0}, {"$B", 1}}}},
		{format: "$A $B !C $E", wantFormat: csvFormat{maxIndex: 3, fields: []csvField{{"$A", 0}, {"$B", 1}, {"$E", 3}}}},
		{format: "!A !B !C $E", wantFormat: csvFormat{maxIndex: 3, fields: []csvField{{"$E", 3}}}},
		{format: "$A $OFFSET $B", wantFormat: csvFormat{maxIndex: 3, fields: []csvField{{"$A", 0}, {"$B", 3}}}},
		{format: "$A $OFFSET $B $OFFSET !A", wantFormat: csvFormat{maxIndex: 3, fields: []csvField{{"$A", 0}, {"$B", 3}}}},
		{format: "$A $OFFSET $OFFSET $B", wantFormat: csvFormat{maxIndex: 5, fields: []csvField{{"$A", 0}, {"$B", 5}}}},
		{format: "$OFFSET $A $OFFSET $B", wantFormat: csvFormat{maxIndex: 5, fields: []csvField{{"$A", 2}, {"$B", 5}}}},
		{format: "$A \"$A", wantErr: true},
		{format: "$A $A", wantErr: true},
		{format: "!A !A", wantErr: true},
		{format: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			c := testCSVConfig
			c.Format = tt.format
			c.checkField = testCheckCSVFormatField
			tt.wantFormat.raw = tt.format

			f, err := newCSVFormat(c)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, f)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantFormat, *f)
			}
		})
	}
}

func TestCSVParser_ReadLine(t *testing.T) {
	tests := []struct {
		name         string
		row          string
		format       string
		wantErr      bool
		wantParseErr bool
	}{
		{name: "match and no error", row: "1 2 3", format: `$A $B $C`},
		{name: "match but error on assigning", row: "1 2 3", format: `$A $B $ERR`, wantErr: true, wantParseErr: true},
		{name: "not match", row: "1 2 3", format: `$A $B $C $d`, wantErr: true, wantParseErr: true},
		{name: "error on reading csv.Err", row: "1 2\"3", format: `$A $B $C`, wantErr: true, wantParseErr: true},
		{name: "error on reading EOF", row: "", format: `$A $B $C`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			r := strings.NewReader(tt.row)
			c := testCSVConfig
			c.Format = tt.format
			p, err := NewCSVParser(c, r)
			require.NoError(t, err)

			err = p.ReadLine(&line)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantParseErr {
					assert.True(t, IsParseError(err))
				} else {
					assert.False(t, IsParseError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCSVParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		row     string
		format  string
		wantErr bool
	}{
		{name: "match and no error", row: "1 2 3", format: `$A $B $C`},
		{name: "match but error on assigning", row: "1 2 3", format: `$A $B $ERR`, wantErr: true},
		{name: "not match", row: "1 2 3", format: `$A $B $C $d`, wantErr: true},
		{name: "error on reading csv.Err", row: "1 2\"3", format: `$A $B $C`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			r := strings.NewReader(tt.row)
			c := testCSVConfig
			c.Format = tt.format
			p, err := NewCSVParser(c, r)
			require.NoError(t, err)

			err = p.ReadLine(&line)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, IsParseError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}

}

func TestCSVParser_Info(t *testing.T) {
	p, err := NewCSVParser(testCSVConfig, nil)
	require.NoError(t, err)
	assert.NotZero(t, p.Info())
}

func testCheckCSVFormatField(name string) (newName string, offset int, valid bool) {
	if len(name) < 2 || !strings.HasPrefix(name, "$") {
		return "", 0, false
	}
	if name == "$OFFSET" {
		return "", 1, false
	}
	return name, 0, true
}
