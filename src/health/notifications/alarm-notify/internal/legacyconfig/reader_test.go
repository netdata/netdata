// SPDX-License-Identifier: GPL-3.0-or-later

package legacyconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAndEvaluate(t *testing.T) {
	tests := map[string]struct {
		sources []string
		initial map[string]string
		want    Settings
	}{
		"empty": {sources: []string{"# a comment\n\n"}},
		"scalar assignments": {
			sources: []string{`SEND_EMAIL=YES EMPTY=; PORT=6667`},
			want:    Settings{Variables: map[string]string{"SEND_EMAIL": "YES", "EMPTY": "", "PORT": "6667"}},
		},
		"quotes and adjacent parts": {
			sources: []string{`A=one\ two B='a $B \ b' C="a ' b" D=un'quo'"ted"`},
			want:    Settings{Variables: map[string]string{"A": "one two", "B": `a $B \ b`, "C": "a ' b", "D": "unquoted"}},
		},
		"double quoted escapes": {
			sources: []string{"A=\"\\$host \\\" \\\\ \\` \\q\""},
			want:    Settings{Variables: map[string]string{"A": "$host \" \\ ` \\q"}},
		},
		"continuations and embedded newlines": {
			sources: []string{"A=one\\\ntwo B=\"one\\\ntwo\" C='one\ntwo'"},
			want:    Settings{Variables: map[string]string{"A": "onetwo", "B": "onetwo", "C": "one\ntwo"}},
		},
		"unicode": {
			sources: []string{"ICON='💚' MSG=\"告警 $ICON\""},
			want:    Settings{Variables: map[string]string{"ICON": "💚", "MSG": "告警 💚"}},
		},
		"assignment order": {
			sources: []string{`A=old B="$A" A=new C=${A} D=$UNDEFINED`},
			want:    Settings{Variables: map[string]string{"A": "new", "B": "old", "C": "new", "D": ""}},
		},
		"no recursive expansion or native secret interpretation": {
			sources: []string{`TOKEN='${file:/synthetic/secret}' COPY="$TOKEN" CMD='$(touch never)' VALUE=$CMD`},
			want: Settings{Variables: map[string]string{
				"TOKEN": "${file:/synthetic/secret}", "COPY": "${file:/synthetic/secret}", "CMD": "$(touch never)", "VALUE": "$(touch never)",
			}},
		},
		"explicit invocation variables": {
			initial: map[string]string{"host": "node-1", "status": "WARNING"},
			sources: []string{`TEXT="${status} on $host"`},
			want:    Settings{Variables: map[string]string{"host": "node-1", "status": "WARNING", "TEXT": "WARNING on node-1"}},
		},
		"overlay replaces values and keeps unrelated keys": {
			sources: []string{
				`DEFAULT_RECIPIENT_EMAIL=old; role_recipients_email[sysadmin]="$DEFAULT_RECIPIENT_EMAIL"; role_recipients_email[dba]=db`,
				`DEFAULT_RECIPIENT_EMAIL=new; role_recipients_email[sysadmin]=override`,
			},
			want: Settings{
				Variables:  map[string]string{"DEFAULT_RECIPIENT_EMAIL": "new"},
				Recipients: map[string]map[string]string{"role_recipients_email": {"sysadmin": "override", "dba": "db"}},
			},
		},
		"recipient assignment is a snapshot": {
			sources: []string{`DEFAULT=old; role_recipients_email[sysadmin]=$DEFAULT`, `DEFAULT=new`},
			want: Settings{Variables: map[string]string{"DEFAULT": "new"}, Recipients: map[string]map[string]string{
				"role_recipients_email": {"sysadmin": "old"},
			}},
		},
		"literal recipient keys and modifiers": {
			sources: []string{`role_recipients_email["ops team"]='a@example.com|critical b@example.com|noclear'; role_recipients_email['db-admin']=''`},
			want: Settings{Recipients: map[string]map[string]string{"role_recipients_email": {
				"ops team": "a@example.com|critical b@example.com|noclear", "db-admin": "",
			}}},
		},
		"quoted recipient padding stays distinct": {
			sources: []string{`role_recipients_email[sysadmin]=plain; role_recipients_email[" sysadmin "]=padded`,
				`role_recipients_email[" sysadmin "]=overlay`},
			want: Settings{Recipients: map[string]map[string]string{"role_recipients_email": {"sysadmin": "plain", " sysadmin ": "overlay"}}},
		},
		"quoted initializer padding stays distinct": {
			sources: []string{`role_recipients_email=([sysadmin]=plain [" sysadmin "]=padded)`},
			want:    Settings{Recipients: map[string]map[string]string{"role_recipients_email": {"sysadmin": "plain", " sysadmin ": "padded"}}},
		},
		"unquoted literal recipient punctuation": {
			sources: []string{`role_recipients_email[db-admin]=db; role_recipients_email[1+2]=ops`},
			want:    Settings{Recipients: map[string]map[string]string{"role_recipients_email": {"db-admin": "db", "1+2": "ops"}}},
		},
		"CRLF settings": {
			sources: []string{"A=ok\r\nB=good\r\n"},
			want:    Settings{Variables: map[string]string{"A": "ok", "B": "good"}},
		},
		"explicit map declaration": {
			sources: []string{`declare -A role_recipients_email; role_recipients_email[sysadmin]=a; declare -A role_recipients_email role_recipients_custom`},
			want:    Settings{Recipients: map[string]map[string]string{"role_recipients_email": {"sysadmin": "a"}, "role_recipients_custom": {}}},
		},
		"keyed initializer and reset": {
			sources: []string{
				`declare -A role_recipients_email=([sysadmin]=a [dba]=b) role_recipients_custom=()`,
				`role_recipients_email=([dba]='override' [empty]=)`,
			},
			want: Settings{Recipients: map[string]map[string]string{
				"role_recipients_email": {"dba": "override", "empty": ""}, "role_recipients_custom": {},
			}},
		},
		"empty map reset": {
			sources: []string{`role_recipients_email[sysadmin]=a; role_recipients_email=()`},
			want:    Settings{Recipients: map[string]map[string]string{"role_recipients_email": {}}},
		},
		"function contents stay inert": {
			sources: []string{"KEY=private\nhelper() { printf '%s' \"$KEY\"; }\ncustom_sender() { local key=$(helper); if true; then curl \"$key\"; fi; }"},
			want: Settings{Variables: map[string]string{"KEY": "private"}, Functions: map[string]string{
				"helper":        `helper() { printf '%s' "$KEY"; }`,
				"custom_sender": `custom_sender() { local key=$(helper); if true; then curl "$key"; fi; }`,
			}},
		},
		"function override": {
			sources: []string{`custom_sender() { echo first; }`, `function custom_sender { echo second; }`},
			want:    Settings{Functions: map[string]string{"custom_sender": `function custom_sender { echo second; }`}},
		},
		"function heredoc and comments": {
			sources: []string{"custom_sender() {\n# internal comment\ncat <<'END'\n} $(still data)\nEND\n}"},
			want:    Settings{Functions: map[string]string{"custom_sender": "custom_sender() {\n# internal comment\ncat <<'END'\n} $(still data)\nEND\n}"}},
		},
		"escaped colon keeps tilde literal": {
			sources: []string{`A=\:~ B=prefix\:~user`},
			want:    Settings{Variables: map[string]string{"A": ":~", "B": "prefix:~user"}},
		},
		"literal assignment metacharacters": {
			sources: []string{`A=* B={a,b} C='~' D=\~ E="~user" F=abc~def`},
			want:    Settings{Variables: map[string]string{"A": "*", "B": "{a,b}", "C": "~", "D": "~", "E": "~user", "F": "abc~def"}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var programs []*Program
			for _, source := range test.sources {
				program, err := Parse(strings.NewReader(source))
				require.NoError(t, err)
				programs = append(programs, program)
			}
			got, err := Evaluate(test.initial, programs...)
			require.NoError(t, err)
			if test.want.Variables == nil {
				test.want.Variables = map[string]string{}
			}
			if test.want.Recipients == nil {
				test.want.Recipients = map[string]map[string]string{}
			}
			if test.want.Functions == nil {
				test.want.Functions = map[string]string{}
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseRejectsUnsupportedSyntax(t *testing.T) {
	tests := map[string]string{
		"command":                         `echo synthetic-private-value`,
		"command substitution":            `KEY=$(cat synthetic-private-value)`,
		"backticks":                       "KEY=`cat synthetic-private-value`",
		"process substitution":            `KEY=<(cat synthetic-private-value)`,
		"conditional":                     `if true; then KEY=synthetic-private-value; fi`,
		"loop":                            `for x in a; do KEY=synthetic-private-value; done`,
		"case":                            `case x in x) KEY=synthetic-private-value;; esac`,
		"source":                          `source synthetic-private-value`,
		"pipeline":                        `KEY=synthetic-private-value | cat`,
		"and list":                        `KEY=synthetic-private-value && true`,
		"redirect":                        `KEY=synthetic-private-value >file`,
		"background":                      `KEY=synthetic-private-value &`,
		"negation":                        `! KEY=synthetic-private-value`,
		"group":                           `{ KEY=synthetic-private-value; }`,
		"subshell":                        `(KEY=synthetic-private-value)`,
		"export":                          `export KEY=synthetic-private-value`,
		"readonly":                        `readonly KEY=synthetic-private-value`,
		"append":                          `KEY+=synthetic-private-value`,
		"arithmetic":                      `KEY=$((1+2))`,
		"default expansion":               `KEY=${VAR:-synthetic-private-value}`,
		"length expansion":                `KEY=${#VAR}`,
		"indirect expansion":              `KEY=${!VAR}`,
		"substring":                       `KEY=${VAR:1:2}`,
		"replacement":                     `KEY=${VAR/a/b}`,
		"special variable":                `KEY=$?`,
		"positional variable":             `KEY=$1`,
		"bare array reference":            `KEY=$role_recipients_email`,
		"array reference":                 `KEY=${role_recipients_email[sysadmin]}`,
		"tilde":                           `KEY=~`,
		"tilde after colon":               `KEY=/prefix:~user`,
		"ansi quote":                      `KEY=$'synthetic-private-value'`,
		"localized quote":                 `KEY=$"synthetic-private-value"`,
		"general array":                   `KEY[0]=synthetic-private-value`,
		"indexed recipient initializer":   `role_recipients_email=(synthetic-private-value)`,
		"scalar recipient map":            `role_recipients_email=synthetic-private-value`,
		"computed recipient key":          `role_recipients_email[$ROLE]=synthetic-private-value`,
		"padded recipient key":            `role_recipients_email[ sysadmin ]=synthetic-private-value`,
		"leading key space":               `role_recipients_email[ sysadmin]=synthetic-private-value`,
		"trailing key space":              `role_recipients_email[sysadmin ]=synthetic-private-value`,
		"padded arithmetic-shaped key":    `role_recipients_email[ db-admin ]=synthetic-private-value`,
		"padded initializer key":          `role_recipients_email=([ sysadmin ]=synthetic-private-value)`,
		"padded declared initializer key": `declare -A role_recipients_email=([ sysadmin ]=synthetic-private-value)`,
		"empty recipient key":             `role_recipients_email['']=synthetic-private-value`,
		"indexed declaration":             `declare -a role_recipients_email`,
		"arbitrary map declaration":       `declare -A OTHER`,
		"declaration flags":               `declare -Ar role_recipients_email`,
		"scalar declaration":              `declare -A role_recipients_email=synthetic-private-value`,
		"nameref":                         `declare -n KEY=synthetic-private-value`,
		"function redirect":               `custom_sender() { :; } >synthetic-private-value`,
		"function subshell body":          `custom_sender() (echo synthetic-private-value)`,
		"function invalid name":           `custom-sender() { :; }`,
		"function invocation":             `custom_sender() { :; }; custom_sender synthetic-private-value`,
		"invalid shell syntax":            `KEY="synthetic-private-value`,
		"nul":                             "KEY='synthetic-private-value\x00'",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			program, err := Parse(strings.NewReader(source))
			require.Error(t, err)
			assert.Nil(t, program)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
		})
	}
}

func TestStockConfiguration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "health_alarm_notify.conf"))
	require.NoError(t, err)
	program, err := Parse(strings.NewReader(string(data)))
	require.NoError(t, err)
	settings, err := Evaluate(map[string]string{
		"status": "WARNING", "host": "node-1", "date": "explicit date", "chart": "system.cpu", "value_string": "90%",
	}, program)
	require.NoError(t, err)
	// Focus on the stock fields defining the reader's compatibility boundary; provider defaults evolve independently.
	assert.Equal(t, "AUTO", settings.Variables["SEND_EMAIL"])
	assert.Equal(t, "WARNING on node-1 at explicit date: system.cpu 90%", settings.Variables["AWSSNS_MESSAGE_FORMAT"])
	assert.Contains(t, settings.Functions["custom_sender"], `urlencode "${msg:0:160}"`)
	assert.Empty(t, settings.Recipients)
}

func TestEvaluationDoesNotExecuteOrUseAmbientEnvironment(t *testing.T) {
	t.Setenv("PR29_AMBIENT_SECRET", "synthetic-private-value")
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "executed"))
	function := fmt.Sprintf("custom_sender() { printf executed >'%s'; }", marker)
	program, err := Parse(strings.NewReader("COPY=$PR29_AMBIENT_SECRET\nLITERAL='${file:/synthetic/secret}'\n" + function))
	require.NoError(t, err)
	initial := map[string]string{"host": "before"}
	settings, err := Evaluate(initial, program)
	require.NoError(t, err)
	assert.Equal(t, Settings{
		Variables:  map[string]string{"host": "before", "COPY": "", "LITERAL": "${file:/synthetic/secret}"},
		Recipients: map[string]map[string]string{}, Functions: map[string]string{"custom_sender": function},
	}, settings)
	assert.NoFileExists(t, marker)
	settings.Variables["host"] = "after"
	assert.Equal(t, map[string]string{"host": "before"}, initial)
	second, err := Evaluate(initial, program)
	require.NoError(t, err)
	assert.Equal(t, "before", second.Variables["host"], "programs can be evaluated independently")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("synthetic-private-value") }

func TestReadErrors(t *testing.T) {
	program, err := Parse(failingReader{})
	assert.Nil(t, program)
	assert.EqualError(t, err, "could not read legacy configuration")
	program, err = Parse(strings.NewReader(strings.Repeat(" ", maxSourceSize+1)))
	assert.Nil(t, program)
	assert.EqualError(t, err, "legacy configuration exceeds the 1 MiB limit")
}

func TestEvaluationLimitsAndErrors(t *testing.T) {
	tests := map[string]struct {
		initial map[string]string
		source  string
		want    string
	}{
		"value growth":                 {source: "A=x;" + strings.Repeat("A=$A$A;", 21), want: "expanded value exceeds"},
		"state growth":                 {initial: map[string]string{"A": strings.Repeat("x", maxValueSize-10)}, source: `B=$A C=$A D=$A E=$A`, want: "evaluated configuration exceeds"},
		"initial value bound":          {initial: map[string]string{"A": strings.Repeat("x", maxValueSize+1)}, want: "expanded value exceeds"},
		"initial scalar map collision": {initial: map[string]string{"role_recipients_email": "x"}, want: "initial scalar variables"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			program, err := Parse(strings.NewReader(test.source))
			require.NoError(t, err)
			settings, err := Evaluate(test.initial, program)
			require.ErrorContains(t, err, test.want)
			assert.Equal(t, Settings{}, settings)
		})
	}
}

func TestRecipientKeyContinuations(t *testing.T) {
	forms := map[string]string{
		"entry":                "role_recipients_email[%s]=value",
		"initializer":          "role_recipients_email=([%s]=value)",
		"declared initializer": "declare -A role_recipients_email=([%s]=value)",
	}
	keys := map[string]struct {
		source string
		want   string
		reject bool
	}{
		"leading continuation":              {source: "\\\nfoo", want: "foo"},
		"trailing continuation":             {source: "foo\\\n", want: "foo"},
		"multiple continuations":            {source: "\\\n\\\nfoo\\\n", want: "foo"},
		"leading CRLF continuation":         {source: "\\\r\nfoo", want: "foo"},
		"middle continuation":               {source: "fo\\\no", want: "foo"},
		"quoted padding after continuation": {source: "\\\n\" foo \"", want: " foo "},
		"punctuation after continuation":    {source: "\\\ndb-admin", want: "db-admin"},
		"space after continuation":          {source: "\\\n foo", reject: true},
		"space before continuation":         {source: " \\\nfoo", reject: true},
		"space after trailing continuation": {source: "foo\\\n ", reject: true},
	}
	for form, format := range forms {
		t.Run(form, func(t *testing.T) {
			for name, key := range keys {
				t.Run(name, func(t *testing.T) {
					program, err := Parse(strings.NewReader(fmt.Sprintf(format, key.source)))
					if key.reject {
						require.Error(t, err)
						assert.Nil(t, program)
						return
					}
					require.NoError(t, err)
					got, err := Evaluate(nil, program)
					require.NoError(t, err)
					assert.Equal(t, Settings{
						Variables: map[string]string{}, Functions: map[string]string{},
						Recipients: map[string]map[string]string{"role_recipients_email": {key.want: "value"}},
					}, got)
				})
			}
		})
	}
}
