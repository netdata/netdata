// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRunTeamsMatrix(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}, "matrix": {}} {
		for source := range map[string]struct{}{"literal": {}, "env": {}, "file": {}} {
			for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
				t.Run(provider+"/"+source+"/"+status, func(t *testing.T) {
					type request struct{ Method, Path, Query, ContentType, Accept, UserAgent, Authorization, Body string }
					requests := make(chan request, 8)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						data, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						requests <- request{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("Accept"), r.Header.Get("User-Agent"), r.Header.Get("Authorization"), string(data)}
						if provider == "matrix" {
							w.WriteHeader(200)
							_, _ = io.WriteString(w, `{"event_id":"$synthetic-event"}`)
						} else {
							w.WriteHeader(202)
						}
					}))
					defer server.Close()
					dst := teamsMatrixTestDestination(provider)
					fields := map[string]*string{"url": &dst.URL}
					if provider == "matrix" {
						dst.APIURL = server.URL + "/proxy/"
						fields = map[string]*string{"url": &dst.APIURL, "token": &dst.AccessToken}
					} else {
						dst.URL = server.URL + "/teams?sig=synthetic-url-secret"
					}
					for name, value := range fields {
						switch source {
						case "env":
							env := "CHAT_TEST_" + strings.ToUpper(name)
							t.Setenv(env, " \n"+*value+"\n")
							*value = "${env:" + env + "}"
						case "file":
							path := filepath.Join(t.TempDir(), name)
							require.NoError(t, os.WriteFile(path, []byte(*value+"\n"), 0600))
							*value = "${file:" + path + "}"
						}
					}
					second, unused := dst, dst
					if provider == "matrix" {
						second.RoomID = "!second:example.org"
						second.AccessToken = "second-synthetic-token"
						unused.APIURL = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
					} else {
						second.URL = server.URL + "/second?sig=second-url-secret"
						unused.URL = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
					}
					cfg := Config{
						Version:      1,
						Destinations: map[string]Destination{"target": dst, "second": second, "unused": unused},
						Routing: Routing{
							Roles:   map[string][]string{"ops": {"target", "second", "target"}},
							Default: []string{"target"},
						},
					}
					config, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					path := writeConfig(t, string(config))
					input, err := json.Marshal(teamsMatrixTestEvent(status, "full"))
					require.NoError(t, err)
					transactions := map[string]struct{}{}
					for repeat := 0; repeat < 2; repeat++ {
						var stdout, stderr bytes.Buffer
						code := Run(
							context.Background(),
							[]string{"send", "--config", path, "--role", "ops", "--role", "unknown"},
							bytes.NewReader(input),
							&stdout,
							&stderr,
						)
						require.Zero(t, code, stderr.String())
						assert.Empty(t, stdout.String())
						assert.Contains(t, stderr.String(), "2 succeeded, 0 failed")
						require.Len(t, requests, 2)
						for index := 0; index < 2; index++ {
							got := <-requests
							assert.JSONEq(t, teamsMatrixTestFixture(t, provider, status, "full"), got.Body)
							got.Body = ""
							want := request{
								Method:      "POST",
								Path:        "/teams",
								Query:       "sig=synthetic-url-secret",
								ContentType: "application/json",
								UserAgent:   "netdata-alarm-notify",
							}
							if index == 1 {
								want.Path, want.Query = "/second", "sig=second-url-secret"
							}
							if provider == "matrix" {
								last := strings.LastIndexByte(got.Path, '/')
								require.Greater(t, last, 0)
								txn := got.Path[last+1:]
								assert.Regexp(t, `^nd_[A-Z2-7]{26,}$`, txn)
								assert.NotContains(t, transactions, txn)
								transactions[txn] = struct{}{}
								got.Path = got.Path[:last] + "/[transaction]"
								want.Method, want.Query, want.Accept = "PUT", "", "application/json"
								want.Path = "/proxy/_matrix/client/v3/rooms/%21room:example.org/send/m.room.message/[transaction]"
								want.Authorization = "Bearer synthetic-token"
								if index == 1 {
									want.Path = "/proxy/_matrix/client/v3/rooms/%21second:example.org/send/m.room.message/[transaction]"
									want.Authorization = "Bearer second-synthetic-token"
								}
							}
							assert.Equal(t, want, got)
						}
						for _, secret := range []string{"synthetic-token", "second-synthetic-token", "synthetic-url-secret", "second-url-secret", server.URL} {
							assert.NotContains(t, stderr.String(), secret)
						}
					}
				})
			}
		}
	}
}

func TestRunMatrixRoomEscaping(t *testing.T) {
	for room, escaped := range map[string]string{"!opaque/hash+id": "%21opaque%2Fhash+id", "!room/?#%:example.org": "%21room%2F%3F%23%25:example.org", "!κ:example.org": "%21%CE%BA:example.org"} {
		t.Run(room, func(t *testing.T) {
			paths := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				paths <- r.URL.EscapedPath()
				_, _ = io.WriteString(w, `{"event_id":"$id"}`)
			}))
			defer server.Close()
			dst := teamsMatrixTestDestination("matrix")
			dst.APIURL, dst.RoomID = server.URL, room
			config, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			require.Zero(
				t,
				Run(
					context.Background(),
					[]string{"send", "--config", writeConfig(t, string(config)), "--destination", "target"},
					strings.NewReader(validEvent),
					&stdout,
					&stderr,
				),
				stderr.String(),
			)
			assert.True(t, strings.HasPrefix(<-paths, "/_matrix/client/v3/rooms/"+escaped+"/send/m.room.message/nd_"))
		})
	}
}

func TestRunTeamsMatrixFailures(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}, "matrix": {}} {
		for name, test := range map[string]struct {
			status    int
			body, err string
			mixed     bool
		}{
			"unauthorized": {401, "synthetic-private-value", "HTTP 401", false}, "forbidden": {403, "", "HTTP 403", false}, "rate limit": {429, "", "HTTP 429", false}, "server": {500, "", "HTTP 500", false},
			"redirect": {307, "", "HTTP 307", false}, "mixed": {400, "synthetic-private-value", "HTTP 400", true},
			"body": {200, "synthetic-private-value", "invalid", false},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				var target, after, redirect atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					switch r.URL.Path {
					case "/after":
						after.Add(1)
						w.WriteHeader(204)
					case "/redirect":
						redirect.Add(1)
						w.WriteHeader(204)
					default:
						target.Add(1)
						w.Header().Set("Location", "/redirect")
						w.WriteHeader(test.status)
						_, _ = io.WriteString(w, test.body)
					}
				}))
				defer server.Close()
				dst := teamsMatrixTestDestination(provider)
				if provider == "matrix" {
					dst.APIURL = server.URL
				} else {
					dst.URL = server.URL
				}
				cfg := Config{
					Version: 1,
					Destinations: map[string]Destination{
						"target": dst,
						"after":  {Type: "webhook", URL: server.URL + "/after"},
					},
					Routing: Routing{Roles: map[string][]string{"ops": {"target", "after"}}},
				}
				config, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				args := []string{"send", "--config", writeConfig(t, string(config)), "--destination", "target"}
				if test.mixed {
					args[len(args)-2], args[len(args)-1] = "--role", "ops"
				}
				var stdout, stderr bytes.Buffer
				code := Run(context.Background(), args, strings.NewReader(validEvent), &stdout, &stderr)
				if test.mixed || provider == "msteams" && name == "body" {
					assert.Zero(t, code)
				} else {
					assert.Equal(t, 1, code)
				}
				if !(provider == "msteams" && name == "body") {
					assert.Contains(t, stderr.String(), test.err)
				}
				if test.mixed {
					assert.EqualValues(t, 1, after.Load())
				} else {
					assert.Zero(t, after.Load())
				}
				assert.EqualValues(t, 1, target.Load())
				assert.Zero(t, redirect.Load())
				assert.Empty(t, stdout.String())
				for _, secret := range []string{"synthetic-token", "synthetic-private-value", server.URL} {
					assert.NotContains(t, stderr.String(), secret)
				}
			})
		}
	}
}

func TestRunTeamsMatrixResolvedSecrets(t *testing.T) {
	for name, test := range map[string]struct{ provider, field, value, err string }{
		"Teams empty URL": {"msteams", "url", " \n", "empty value"}, "Teams relative URL": {"msteams", "url", "/synthetic-private-value", "absolute HTTP(S)"},
		"Teams userinfo":     {"msteams", "url", "https://user:synthetic-private-value@example.com", "user information"},
		"Matrix empty token": {"matrix", "token", " \n", "empty value"}, "Matrix control token": {"matrix", "token", "synthetic-private-value\nother", "without whitespace"},
		"Matrix nested token": {"matrix", "token", "${env:NESTED}", "without whitespace"}, "Matrix URL query": {"matrix", "url", "https://example.com/?synthetic-private-value", "query"},
		"Matrix official HTTP": {"matrix", "url", "http://matrix.org", "HTTPS"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }),
			)
			defer server.Close()
			dst := teamsMatrixTestDestination(test.provider)
			t.Setenv("CHAT_INVALID_SECRET", test.value)
			if test.provider == "msteams" {
				dst.URL = "${env:CHAT_INVALID_SECRET}"
			} else if test.field == "url" {
				dst.APIURL = "${env:CHAT_INVALID_SECRET}"
				dst.AccessToken = "${file:" + filepath.Join(t.TempDir(), "absent") + "}"
			} else {
				dst.APIURL = server.URL
				dst.AccessToken = "${env:CHAT_INVALID_SECRET}"
			}
			config, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"send", "--config", writeConfig(t, string(config)), "--destination", "target"},
				strings.NewReader(validEvent),
				&stdout,
				&stderr,
			)
			assert.Equal(t, 1, code)
			assert.Zero(t, calls.Load())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.err)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), "could not read secret file")
		})
	}
}

func TestRunMSTeamsPayloadLimit(t *testing.T) {
	for name, symbol := range map[string]string{"ASCII": "x", "Unicode": "界", "JSON escaping": "\"", "Markdown": "*", "HTML": "<"} {
		for suffix, extra := range map[string]int{"boundary": 0, "over": 1} {
			t.Run(name+"/"+suffix, func(t *testing.T) {
				sizes := make(chan int, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, _ := io.ReadAll(r.Body)
					sizes <- len(data)
					w.WriteHeader(202)
				}))
				defer server.Close()
				dst := teamsMatrixTestDestination("msteams")
				dst.URL = server.URL
				event := teamsMatrixTestEvent("WARNING", "minimal")
				event.Info = "x"
				baseline, err := json.Marshal(renderMSTeams(dst, event))
				require.NoError(t, err)
				event.Info = symbol
				sample, err := json.Marshal(renderMSTeams(dst, event))
				require.NoError(t, err)
				width := len(sample) - len(baseline) + 1
				remaining := msTeamsPayloadLimit - len(baseline) + 1
				event.Info = strings.Repeat(symbol, remaining/width) + strings.Repeat("x", remaining%width+extra)
				input, err := json.Marshal(event)
				require.NoError(t, err)
				config, err := yaml.Marshal(Config{Version: 1, Destinations: map[string]Destination{"target": dst}})
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				code := Run(
					context.Background(),
					[]string{"send", "--config", writeConfig(t, string(config)), "--destination", "target"},
					bytes.NewReader(input),
					&stdout,
					&stderr,
				)
				if extra == 0 {
					require.Zero(t, code, stderr.String())
					require.Len(t, sizes, 1)
					assert.Equal(t, msTeamsPayloadLimit, <-sizes)
				} else {
					assert.Equal(t, 1, code)
					assert.Empty(t, sizes)
					assert.Contains(t, stderr.String(), "28 KiB")
				}
			})
		}
	}
}
