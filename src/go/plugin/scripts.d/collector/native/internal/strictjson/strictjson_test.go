// SPDX-License-Identifier: GPL-3.0-or-later

package strictjson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	message := Object(Fields{
		"labels": Map(Scalar()),
		"items":  Object(nil, "id"),
		"data":   Any(),
	}, "version")
	metadata := Optional(Fields{
		"items": Optional(nil, "id"),
	}, "version")
	tests := map[string]struct {
		shape   *Shape
		data    string
		wantErr error
	}{
		"exact fields": {
			shape: message,
			data:  `{"version":"v1","items":[{"id":"a"}],"labels":{"Any-Key":"x"}}`,
		},
		"arbitrary data":          {shape: message, data: `{"data":{"x":[null,{"y":null}],"Version":1}}`},
		"arrays are transparent":  {shape: message, data: `{"items":[[{"id":"a"}]]}`},
		"unknown field":           {shape: message, data: `{"surprise":1}`, wantErr: errUnknownField},
		"case-folded field":       {shape: message, data: `{"Version":"v1"}`, wantErr: errUnknownField},
		"nested unknown field":    {shape: message, data: `{"items":[{"id":"a","ID":"b"}]}`, wantErr: errUnknownField},
		"scalar with object keys": {shape: message, data: `{"version":{"x":1}}`, wantErr: errUnknownField},
		"duplicate key":           {shape: message, data: `{"version":"v1","version":"v1"}`, wantErr: errDuplicateKey},
		"escaped duplicate key": {
			shape:   message,
			data:    `{"version":"v1","` + `\` + "u0076" + `ersion":"v2"}`,
			wantErr: errDuplicateKey,
		},
		"duplicate in any":       {shape: message, data: `{"data":{"x":1,"x":2}}`, wantErr: errDuplicateKey},
		"null value":             {shape: message, data: `{"version":null}`, wantErr: errUnexpectedNull},
		"null label":             {shape: message, data: `{"labels":{"a":null}}`, wantErr: errUnexpectedNull},
		"null array element":     {shape: message, data: `{"items":[null]}`, wantErr: errUnexpectedNull},
		"null document":          {shape: message, data: `null`, wantErr: errUnexpectedNull},
		"optional nulls":         {shape: metadata, data: `{"version":null,"items":[null,{"id":null}]}`},
		"optional null document": {shape: metadata, data: `null`},
		"optional unknown field": {shape: metadata, data: `{"items":[{"id":null,"x":1}]}`, wantErr: errUnknownField},
		"invalid UTF-8":          {shape: message, data: "{\"version\":\"\xff\"}", wantErr: errInvalidUTF8},
		"truncated":              {shape: message, data: `{"version":"v1"`, wantErr: errInvalidJSON},
		"trailing value":         {shape: message, data: `{} {}`, wantErr: errInvalidJSON},
		"excessive nesting": {
			shape:   Any(),
			data:    strings.Repeat("[", 20000) + strings.Repeat("]", 20000),
			wantErr: errInvalidJSON,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validate([]byte(tc.data), tc.shape)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestValidate_ErrorsOmitValues(t *testing.T) {
	err := validate([]byte(`{"version":1e999,"SYNTHETIC_SECRET":1}`), Object(nil, "version"))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "1e999")
	assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET")
}

func TestDecode(t *testing.T) {
	shape := Object(Fields{
		"data": Any(),
	}, "version")
	type target struct {
		Version string `json:"version"`
		Data    any    `json:"data"`
	}
	tests := map[string]struct {
		decode  func([]byte, *Shape, any) error
		data    string
		want    target
		wantErr error
	}{
		"decode": {
			decode: Decode,
			data:   `{"version":"v1","data":[9007199254740993]}`,
			want: target{
				Version: "v1",
				Data:    []any{float64(9007199254740993)},
			},
		},
		"decode use number": {
			decode: DecodeUseNumber,
			data:   `{"version":"v1","data":[9007199254740993]}`,
			want: target{
				Version: "v1",
				Data:    []any{json.Number("9007199254740993")},
			},
		},
		"shape error": {decode: Decode, data: `{"Version":"v1"}`, wantErr: errUnknownField},
		"type error":  {decode: Decode, data: `{"version":1}`, wantErr: errTargetSchema},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got target
			err := tc.decode([]byte(tc.data), shape, &got)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
