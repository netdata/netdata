// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInputCapsulePreservesLiteralFields(t *testing.T) {
	for name, test := range map[string]struct {
		command string
		source  string
		args    []string
	}{
		"inner NBSP":         {command: "echo db\u00a0one", args: []string{"db\u00a0one"}},
		"trailing NBSP":      {command: "echo db\u00a0", args: []string{"db\u00a0"}},
		"Unicode spaces":     {command: "echo db\u2003one db\u202f", args: []string{"db\u2003one", "db\u202f"}},
		"literal escapes":    {command: `echo d\x62 d\u0062 db\n`, args: []string{`d\x62`, `d\u0062`, `db\n`}},
		"Windows path":       {command: `echo C:\users\db`, args: []string{`C:\users\db`}},
		"trailing backslash": {command: `echo db\`, source: `user=domain\`, args: []string{`db\`}},
		"source escapes":     {command: "echo db", source: `user=domain\name,tag=\x62`, args: []string{"db"}},
		"apostrophe equals":  {command: "echo filter=O'Brien", source: "user=O'Brien", args: []string{"filter=O'Brien"}},
		"ASCII whitespace":   {command: "echo\tfirst\rsecond\vthird\ffourth  ", args: []string{"first", "second", "third", "fourth"}},
		"safe Unicode":       {command: "echo db-α db+one", source: "user=α", args: []string{"db-α", "db+one"}},
	} {
		t.Run(name, func(t *testing.T) {
			// The daemon quotes sanitized fields literally, without Go/JSON escaping.
			payload := []byte("{\"value\":\"db\u00a0\\\\literal\"}\n")
			input := "FUNCTION_PAYLOAD literal 30 \"" + test.command + "\" \"0xFFFF\" \"" + test.source + "\" \"application/json\"\n" +
				string(payload) + "\nFUNCTION_PAYLOAD_END\n" +
				"FUNCTION next 30 \"echo next\" 0xFFFF \"\"\nQUIT\n"
			consumer := &recordingCapsuleConsumer{}
			capsule, err := NewInputCapsule(strings.NewReader(input))
			require.NoError(t, err)
			require.NoError(t, capsule.Run(context.Background(), consumer))
			require.Equal(t, []Call{{
				UID: "literal", Timeout: 30 * time.Second, Method: "echo", Args: test.args,
				Access: "0xFFFF", Source: test.source, HasPayload: true, ContentType: "application/json", Payload: payload,
			}, {
				UID: "next", Timeout: 30 * time.Second, Method: "echo", Args: []string{"next"}, Access: "0xFFFF",
			}}, consumer.calls)
			require.Empty(t, consumer.rejections)
			require.True(t, consumer.quit)
		})
	}
}

func TestInputCapsuleRejectsMalformedFieldQuotes(t *testing.T) {
	for name, line := range map[string]string{
		"unterminated":      `FUNCTION bad 30 "echo arg`,
		"missing separator": `FUNCTION bad 30 "echo arg"suffix 0xFFFF ""`,
		"embedded quote":    `FUNCTION bad 30 "echo a"b" 0xFFFF ""`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer := &recordingCapsuleConsumer{}
			capsule, err := NewInputCapsule(strings.NewReader(line + "\nFUNCTION next 30 \"echo next\" 0xFFFF \"\"\nQUIT\n"))
			require.NoError(t, err)
			require.NoError(t, capsule.Run(context.Background(), consumer))
			require.Equal(t, []recordedCapsuleRejection{{uid: "bad", status: 400}}, consumer.rejections)
			require.Equal(t, []Call{{UID: "next", Timeout: 30 * time.Second, Method: "echo", Args: []string{"next"}, Access: "0xFFFF"}}, consumer.calls)
			require.True(t, consumer.quit)
		})
	}
}
