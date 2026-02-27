// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("valid writer", func(t *testing.T) {
		require.NotNil(t, New(&bytes.Buffer{}))
	})

	t.Run("nil writer", func(t *testing.T) {
		require.Panics(t, func() { New(nil) })
	})
}

func TestChart(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	opts := ChartOpts{
		TypeID:      "system",
		ID:          "cpu",
		Name:        "cpu_system",
		Title:       "CPU Usage",
		Units:       "percentage",
		Family:      "cpu",
		Context:     "system.cpu",
		ChartType:   "line",
		Priority:    1000,
		UpdateEvery: 1,
		Options:     "",
		Plugin:      "system",
		Module:      "cpu",
	}

	api.CHART(opts)

	expected := "CHART 'system.cpu' 'cpu_system' 'CPU Usage' 'percentage' 'cpu' 'system.cpu' " +
		"'line' '1000' '1' '' 'system' 'cpu'\n"

	require.Equal(t, expected, w.String())
}

func TestDimension(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	opts := DimensionOpts{
		ID:         "user",
		Name:       "user",
		Algorithm:  "absolute",
		Multiplier: 1,
		Divisor:    1,
		Options:    "",
	}

	api.DIMENSION(opts)

	expected := "DIMENSION 'user' 'user' 'absolute' '1' '1' ''\n"

	require.Equal(t, expected, w.String())
}

func TestCLABEL(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.CLABEL("key1", "value1", 1)

	expected := "CLABEL 'key1' 'value1' '1'\n"

	require.Equal(t, expected, w.String())
}

func TestCLABELCOMMIT(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.CLABELCOMMIT()

	expected := "CLABEL_COMMIT\n"

	require.Equal(t, expected, w.String())
}

func TestBEGIN(t *testing.T) {

	tests := map[string]struct {
		name     string
		typeID   string
		ID       string
		msSince  int
		expected string
	}{
		"without msSince": {
			typeID:   "system",
			ID:       "cpu",
			msSince:  0,
			expected: "BEGIN 'system.cpu'\n",
		},
		"with msSince": {
			typeID:   "system",
			ID:       "cpu",
			msSince:  1000,
			expected: "BEGIN 'system.cpu' 1000\n",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			w := &bytes.Buffer{}
			api := New(w)

			api.BEGIN(test.typeID, test.ID, test.msSince)

			require.Equal(t, test.expected, w.String())
		})
	}
}

func TestSET(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.SET("cpu_user", 42)

	expected := "SET 'cpu_user' = 42\n"

	require.Equal(t, expected, w.String())
}

func TestSETFLOAT(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.SETFLOAT("cpu_user", 42.42)

	expected := "SET 'cpu_user' = 42.42\n"

	require.Equal(t, expected, w.String())
}

func TestSETEMPTY(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.SETEMPTY("cpu_user")

	expected := "SET 'cpu_user' = \n"

	require.Equal(t, expected, w.String())
}

func TestVARIABLE(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.VARIABLE("var1", 100)
	api.VARIABLE("var2", 100.1)

	expected := "VARIABLE CHART 'var1' = 100\nVARIABLE CHART 'var2' = 100.1\n"

	require.Equal(t, expected, w.String())
}

func TestEND(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.END()

	expected := "END\n\n"

	require.Equal(t, expected, w.String())
}

func TestDISABLE(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.DISABLE()

	expected := "DISABLE\n"

	require.Equal(t, expected, w.String())
}

func TestEMPTYLINE(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	require.NoError(t, api.EMPTYLINE())

	expected := "\n"

	require.Equal(t, expected, w.String())
}

func TestHOSTINFO(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	info := HostInfo{
		GUID:     "test-guid",
		Hostname: "test-host",
		Labels: map[string]string{
			"label1": "value1",
		},
	}

	api.HOSTINFO(info)

	expected := `
HOST_DEFINE 'test-guid' 'test-host'
HOST_LABEL 'label1' 'value1'
HOST_DEFINE_END

`[1:]

	require.Equal(t, expected, w.String())
}

func TestHOST(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.HOST("test-guid")

	expected := "HOST 'test-guid'\n\n"

	require.Equal(t, expected, w.String())
}

func TestFUNCRESULT(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	result := FunctionResult{
		UID:             "test-uid",
		ContentType:     "text/plain",
		Payload:         "test payload",
		Code:            "200",
		ExpireTimestamp: "1234567890",
	}

	api.FUNCRESULT(result)

	expected := "FUNCTION_RESULT_BEGIN test-uid 200 text/plain 1234567890\ntest payload\nFUNCTION_RESULT_END\n\n"

	require.Equal(t, expected, w.String())
}

func TestCONFIGCREATE(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	opts := ConfigOpts{
		ID:                "test-config",
		Status:            "active",
		ConfigType:        "test",
		Path:              "/test/path",
		SourceType:        "file",
		Source:            "test.conf",
		SupportedCommands: "read,write",
	}

	api.CONFIGCREATE(opts)

	expected := "CONFIG test-config create active test /test/path file 'test.conf' 'read,write' 0x0000 0x0000\n\n"

	require.Equal(t, expected, w.String())
}

func TestCONFIGDELETE(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.CONFIGDELETE("test-config")

	expected := "CONFIG test-config delete\n\n"

	require.Equal(t, expected, w.String())
}

func TestCONFIGSTATUS(t *testing.T) {
	w := &bytes.Buffer{}
	api := New(w)

	api.CONFIGSTATUS("test-config", "inactive")

	expected := "CONFIG test-config status inactive\n\n"

	require.Equal(t, expected, w.String())
}
