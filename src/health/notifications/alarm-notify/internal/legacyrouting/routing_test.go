// SPDX-License-Identifier: GPL-3.0-or-later

package legacyrouting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readSettings(t *testing.T, sources ...string) legacyconfig.Settings {
	t.Helper()
	var programs []*legacyconfig.Program
	for _, source := range sources {
		program, err := legacyconfig.Parse(strings.NewReader(source))
		require.NoError(t, err)
		programs = append(programs, program)
	}
	settings, err := legacyconfig.Evaluate(nil, programs...)
	require.NoError(t, err)
	return settings
}

func TestResolve(t *testing.T) {
	for name, test := range map[string]struct {
		sources []string
		methods []string
		roles   []string
		want    []Target
	}{
		"defaults for missing and empty roles": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL='fallback'; role_recipients_email[empty]=''; role_recipients_email[ops]=specific`},
			methods: []string{"email"}, roles: []string{"missing empty ops"},
			want: []Target{{"email", "fallback"}, {"email", "specific"}},
		},
		"only exactly empty entries fall back": {
			sources: []string{"DEFAULT_RECIPIENT_EMAIL=fallback; role_recipients_email[ops]=' ,\t\n'"},
			methods: []string{"email"}, roles: []string{"ops"},
		},
		"overlays and assignment-time copies": {
			sources: []string{
				`DEFAULT_RECIPIENT_EMAIL=old; role_recipients_email[ops]=$DEFAULT_RECIPIENT_EMAIL; role_recipients_email[db]=retained`,
				`DEFAULT_RECIPIENT_EMAIL=new; role_recipients_email[other]=changed`,
			},
			methods: []string{"email"}, roles: []string{"ops db missing other"},
			want: []Target{{"email", "old"}, {"email", "retained"}, {"email", "new"}, {"email", "changed"}},
		},
		"role and recipient separators": {
			sources: []string{"role_recipients_email[ops]='a,b\tc\nd'; role_recipients_email[db]='d,e'"},
			methods: []string{"email"}, roles: []string{"ops,\tdb\nops", "db"},
			want: []Target{{"email", "a"}, {"email", "b"}, {"email", "c"}, {"email", "d"}, {"email", "e"}},
		},
		"shell IFS only": {
			sources: []string{"role_recipients_email['ops\u00a0db']='a\u00a0b,c\rd,e\vf'"},
			methods: []string{"email"}, roles: []string{"ops\u00a0db"},
			want: []Target{{"email", "a\u00a0b"}, {"email", "c\rd"}, {"email", "e\vf"}},
		},
		"reserved roles do not suppress other roles": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL=fallback; role_recipients_email[silent]=bad; role_recipients_email[disabled]=bad`},
			methods: []string{"email"}, roles: []string{"silent disabled ops"}, want: []Target{{"email", "fallback"}},
		},
		"all roles reserved": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL=fallback`},
			methods: []string{"email"}, roles: []string{"silent,disabled"},
		},
		"role and disabled marker case is exact": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL='disabled DISABLED silent'; role_recipients_email[ops]=specific`},
			methods: []string{"email"}, roles: []string{"OPS SILENT"},
			want: []Target{{"email", "DISABLED"}, {"email", "silent"}},
		},
		"disabled role entry does not fall back": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL=fallback; role_recipients_email[ops]=disabled`},
			methods: []string{"email"}, roles: []string{"ops"},
		},
		"deduplicate within method only": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL='same same'; DEFAULT_RECIPIENT_SMS=same`},
			methods: []string{"sms", "email", "sms"}, roles: []string{"ops db"},
			want: []Target{{"sms", "same"}, {"email", "same"}},
		},
		"first appearance includes a filtered occurrence": {
			sources: []string{`role_recipients_email[ops]='first|nowarn second'; role_recipients_email[db]=first`},
			methods: []string{"email"}, roles: []string{"ops db"},
			want: []Target{{"email", "first"}, {"email", "second"}},
		},
		"unselected settings are not inspected": {
			sources: []string{`role_recipients_sms[ops]='secret|invalid'; role_recipients_email[other]='secret|invalid'; role_recipients_email[ops]=selected`},
			methods: []string{"email"}, roles: []string{"ops"}, want: []Target{{"email", "selected"}},
		},
		"empty modifier segments and duplicates": {
			sources: []string{`DEFAULT_RECIPIENT_EMAIL='a||NoClear|noclear| b|'`},
			methods: []string{"email"}, roles: []string{"ops"}, want: []Target{{"email", "a"}, {"email", "b"}},
		},
		"applicable methods are supplied by caller": {
			sources: []string{`SEND_EMAIL=NO; DEFAULT_RECIPIENT_EMAIL=recipient`},
			methods: []string{"email"}, roles: []string{"ops"}, want: []Target{{"email", "recipient"}},
		},
		"no applicable methods": {sources: []string{`DEFAULT_RECIPIENT_EMAIL=recipient`}, roles: []string{"ops"}},
		"no roles":              {sources: []string{`DEFAULT_RECIPIENT_EMAIL=recipient`}, methods: []string{"email"}},
		"no recipients":         {methods: []string{"email"}, roles: []string{"ops"}},
	} {
		t.Run(name, func(t *testing.T) {
			settings := readSettings(t, test.sources...)
			got, err := Resolve(settings, test.methods, test.roles, notifier.Notification{Event: event.Event{Status: "WARNING"}})
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.Equal(t, readSettings(t, test.sources...), settings, "resolution must not mutate settings")
		})
	}
}

func TestResolvePolicies(t *testing.T) {
	const missing = "critical_seen_since_clear is required"
	// Expected eligibility for unknown, false and true history, independent of the shared predicate.
	for name, test := range map[string]struct {
		modifiers string
		warning   [3]string
		clear     [3]string
	}{
		"none":            {},
		"nowarn":          {modifiers: "|NoWarn", warning: [3]string{"skip", "skip", "skip"}},
		"noclear":         {modifiers: "|NOCLEAR", clear: [3]string{"skip", "skip", "skip"}},
		"both stateless":  {modifiers: "|nowarn|noclear", warning: [3]string{"skip", "skip", "skip"}, clear: [3]string{"skip", "skip", "skip"}},
		"critical":        {modifiers: "|Critical", warning: [3]string{missing, "skip", ""}, clear: [3]string{missing, "skip", ""}},
		"critical nowarn": {modifiers: "|critical|nowarn", warning: [3]string{"skip", "skip", "skip"}, clear: [3]string{missing, "skip", ""}},
		"critical noclear": {modifiers: "|critical|noclear", warning: [3]string{missing, "skip", ""},
			clear: [3]string{"skip", "skip", "skip"}},
		"all": {modifiers: "|critical|nowarn|noclear", warning: [3]string{"skip", "skip", "skip"}, clear: [3]string{"skip", "skip", "skip"}},
	} {
		for status, outcomes := range map[string][3]string{"WARNING": test.warning, "CLEAR": test.clear, "CRITICAL": {}} {
			for history, fact := range map[string]struct {
				value *bool
				index int
			}{"unknown": {}, "false": {new(false), 1}, "true": {new(true), 2}} {
				t.Run(name+"/"+status+"/"+history, func(t *testing.T) {
					settings := readSettings(t, fmt.Sprintf("DEFAULT_RECIPIENT_EMAIL='recipient%s'", test.modifiers))
					got, err := Resolve(settings, []string{"email"}, []string{"ops"}, notifier.Notification{
						Event: event.Event{Status: status}, CriticalSeenSinceClear: fact.value,
					})
					outcome := outcomes[fact.index]
					var want []Target
					if outcome == missing {
						require.ErrorContains(t, err, missing)
					} else {
						require.NoError(t, err)
						if outcome == "" {
							want = []Target{{"email", "recipient"}}
						}
					}
					assert.Equal(t, want, got)
					if fact.value != nil {
						assert.Equal(t, fact.index == 2, *fact.value, "history is input-only")
					}
				})
			}
		}
	}
}

func TestResolvePolicyUnions(t *testing.T) {
	const missing = "critical_seen_since_clear is required"
	for name, test := range map[string]struct {
		first, second string
		warning       [3]string
		clear         [3]string
	}{
		"complementary stateless":  {first: "|nowarn", second: "|noclear"},
		"unrestricted alternative": {first: "|critical"},
		"conditional alternative":  {first: "|critical", second: "|nowarn", warning: [3]string{missing, "skip", ""}},
		"complementary critical": {first: "|critical|nowarn", second: "|critical|noclear",
			warning: [3]string{missing, "skip", ""}, clear: [3]string{missing, "skip", ""}},
		"both stateless skipped": {first: "|critical|nowarn", second: "|nowarn",
			warning: [3]string{"skip", "skip", "skip"}},
	} {
		for status, outcomes := range map[string][3]string{"WARNING": test.warning, "CLEAR": test.clear, "CRITICAL": {}} {
			for history, fact := range map[string]struct {
				value *bool
				index int
			}{"unknown": {}, "false": {new(false), 1}, "true": {new(true), 2}} {
				for order, roles := range map[string][]string{"forward": {"ops", "db"}, "reverse": {"db", "ops"}} {
					t.Run(name+"/"+status+"/"+history+"/"+order, func(t *testing.T) {
						settings := readSettings(t, fmt.Sprintf("role_recipients_email[ops]='same%s'; role_recipients_email[db]='same%s'", test.first, test.second))
						got, err := Resolve(settings, []string{"email"}, roles, notifier.Notification{
							Event: event.Event{Status: status}, CriticalSeenSinceClear: fact.value,
						})
						var want []Target
						if outcomes[fact.index] == missing {
							require.ErrorContains(t, err, missing)
						} else {
							require.NoError(t, err)
							if outcomes[fact.index] == "" {
								want = []Target{{"email", "same"}}
							}
						}
						assert.Equal(t, want, got)
					})
				}
			}
		}
	}
}

func TestResolveErrorsAreAtomicAndSafe(t *testing.T) {
	for name, test := range map[string]struct {
		source  string
		status  string
		methods []string
		err     string
	}{
		"unknown modifier":                     {source: `DEFAULT_RECIPIENT_EMAIL='good synthetic-private-value|typo'`, err: "unknown recipient modifier"},
		"unknown despite permitting duplicate": {source: `DEFAULT_RECIPIENT_EMAIL='synthetic-private-value synthetic-private-value|typo'`, err: "unknown recipient modifier"},
		"unknown on skipped occurrence":        {source: `DEFAULT_RECIPIENT_EMAIL='synthetic-private-value|nowarn|typo'`, err: "unknown recipient modifier"},
		"missing recipient":                    {source: `DEFAULT_RECIPIENT_EMAIL='good |critical'`, err: "recipient must not be empty"},
		"later history error":                  {source: `DEFAULT_RECIPIENT_EMAIL='good synthetic-private-value|critical'`, err: "critical_seen_since_clear is required"},
		"history cannot cross methods":         {source: `DEFAULT_RECIPIENT_EMAIL=recipient; DEFAULT_RECIPIENT_SMS='recipient|critical'`, methods: []string{"email", "sms"}, err: "critical_seen_since_clear is required"},
		"later method error":                   {source: `DEFAULT_RECIPIENT_EMAIL=good; DEFAULT_RECIPIENT_SMS='synthetic-private-value|typo'`, methods: []string{"email", "sms"}, err: "legacy routing method 2: unknown recipient modifier"},
		"invalid status":                       {source: `DEFAULT_RECIPIENT_EMAIL=good`, status: "synthetic-private-value", err: "requires WARNING, CRITICAL or CLEAR"},
	} {
		t.Run(name, func(t *testing.T) {
			status := test.status
			if status == "" {
				status = "WARNING"
			}
			methods := test.methods
			if methods == nil {
				methods = []string{"email"}
			}
			got, err := Resolve(readSettings(t, test.source), methods, []string{"ops"}, notifier.Notification{Event: event.Event{Status: status}})
			require.ErrorContains(t, err, test.err)
			assert.Nil(t, got)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
		})
	}
}

func TestResolveKeepsRecipientsLiteral(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile("unexpected-recipient", []byte("synthetic"), 0600))
	marker := filepath.ToSlash(filepath.Join(dir, "executed"))
	t.Setenv("PR30_RECIPIENT", "unexpected-recipient")
	settings := readSettings(t, fmt.Sprintf("role_recipients_email['*']='* ? [ab] ${env:PR30_RECIPIENT} ${file:/synthetic/token}'; role_recipients_email['unexpected-recipient']=bad; custom_sender() { printf executed >'%s'; }", marker))
	want := []Target{{"email", "*"}, {"email", "?"}, {"email", "[ab]"}, {"email", "${env:PR30_RECIPIENT}"}, {"email", "${file:/synthetic/token}"}}
	for range 2 {
		got, err := Resolve(settings, []string{"email"}, []string{"*"}, notifier.Notification{Event: event.Event{Status: "WARNING"}})
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NoFileExists(t, marker)
	}
}
