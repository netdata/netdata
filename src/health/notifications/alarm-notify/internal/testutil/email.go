// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type EmailPart struct{ Kind, Body string }

func ParseEmail(t *testing.T, raw []byte) (mail.Header, []EmailPart) {
	t.Helper()
	for _, line := range strings.Split(string(raw), "\r\n") {
		require.LessOrEqual(t, len(line), 998, "wire line exceeds RFC 5322 limit")
		require.NotContains(t, line, "\n", "bare LF")
		require.NotContains(t, line, "\r", "bare CR")
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	kind, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	require.NoError(t, err)
	var parts []EmailPart
	readPart := func(kind string, params map[string]string, encoding string, body io.Reader) {
		t.Helper()
		require.Equal(t, map[string]string{"charset": "UTF-8"}, params)
		require.Equal(t, "quoted-printable", encoding)
		decoded, err := io.ReadAll(quotedprintable.NewReader(body))
		require.NoError(t, err)
		parts = append(parts, EmailPart{kind, string(decoded)})
	}
	if kind == "multipart/alternative" {
		require.Len(t, params, 1)
		require.NotEmpty(t, params["boundary"])
		reader := multipart.NewReader(message.Body, params["boundary"])
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			kind, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
			require.NoError(t, err)
			require.Len(t, part.Header, 2)
			readPart(kind, params, part.Header.Get("Content-Transfer-Encoding"), part)
		}
	} else {
		readPart(kind, params, message.Header.Get("Content-Transfer-Encoding"), message.Body)
	}
	return message.Header, parts
}
