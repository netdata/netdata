// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// A template's values may change with target text, but its structure and types must not.
// Substituting a marker in the ordinary result provides an oracle independent of YAML escaping.
func TestDiscoveryTemplatesPreserveStringValues(t *testing.T) {
	const marker = "DISCOVERY_VALUE"
	values := map[string]string{
		"yaml boolean":              "on",
		"yaml number":               "0123",
		"yaml null":                 "null",
		"yaml syntax":               "{value: [a, b]} # comment",
		"quotes":                    "a'\"`\\b",
		"line breaks":               "a\nb\rc\u0085d\u2028e\u2029f",
		"injected keys":             "device\nmodule: unexpected\nurl: unexpected",
		"trailing newlines":         "line\n\n",
		"leading space and newline": " leading\n  indented\n",
		"only newlines":             "\n\n",
		"controls":                  "a\x00\t\x01\x1f\x7f\x9fb",
		"unicode":                   "路由器-é",
		"invalid UTF-8":             string([]byte{'a', 0xff, 'b'}),
	}
	for _, source := range discoveryTemplateSources(t) {
		// Token extraction intentionally stops at spaces; its exact behavior has separate stock-rule tests.
		if strings.Contains(source.body, "--api-key=") {
			continue
		}
		t.Run(source.name, func(t *testing.T) {
			for _, version := range []string{"2c", "3"} {
				baseline := renderDiscoverySource(t, source, serializationTarget(marker, version, source.body))
				for name, value := range values {
					t.Run(version+"/"+name, func(t *testing.T) {
						got := renderDiscoverySource(t, source, serializationTarget(value, version, source.body))
						want := replaceDiscoveryMarker(baseline, marker, value)
						assert.Equal(t, want, got)
					})
				}
			}
		})
	}
}

type discoveryTemplateSource struct{ name, id, body string }

func discoveryTemplateSources(t *testing.T) []discoveryTemplateSource {
	t.Helper()
	var sources []discoveryTemplateSource
	root := "../../../../go.d"
	addRules := func(name string, data any) {
		var visit func(any)
		visit = func(value any) {
			switch value := value.(type) {
			case map[any]any:
				if body, ok := value["config_template"].(string); ok && strings.Contains(body, "{{") {
					id, _ := value["id"].(string)
					sources = append(sources, discoveryTemplateSource{
						name: name + "/" + id,
						id:   id,
						body: body,
					})
				}
				for _, child := range value {
					visit(child)
				}
			case []any:
				for _, child := range value {
					visit(child)
				}
			}
		}
		visit(data)
	}
	files, err := filepath.Glob(filepath.Join(root, "config/go.d/sd/*.conf"))
	require.NoError(t, err)
	for _, path := range files {
		bs, err := os.ReadFile(path)
		require.NoError(t, err)
		var data any
		require.NoError(t, yaml.Unmarshal(bs, &data))
		addRules(filepath.Base(path), data)
		// Stock HTTP's commented examples are executable user-facing templates too.
		for _, block := range strings.Split(string(bs), "\n\n") {
			if !strings.Contains(block, "## - id:") {
				continue
			}
			var lines []string
			started := false
			for _, line := range strings.Split(block, "\n") {
				line = strings.TrimPrefix(line, "  ")
				if strings.HasPrefix(line, "## - id:") {
					started = true
				}
				if started {
					lines = append(lines, strings.TrimPrefix(line, "## "))
				}
			}
			require.NoError(t, yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &data))
			addRules(filepath.Base(path)+"/comment", data)
		}
	}
	files, err = filepath.Glob(filepath.Join(root, "discovery/sdext/config_schema_*.json"))
	require.NoError(t, err)
	for _, path := range files {
		bs, err := os.ReadFile(path)
		require.NoError(t, err)
		var data any
		require.NoError(t, json.Unmarshal(bs, &data))
		var visit func(any, string)
		visit = func(value any, key string) {
			switch value := value.(type) {
			case map[string]any:
				if key == "config_template" {
					for _, field := range []string{"default", "ui:placeholder"} {
						if body, ok := value[field].(string); ok {
							sources = append(sources, discoveryTemplateSource{
								name: filepath.Base(path) + "/" + field,
								id:   "snmp",
								body: body,
							})
						}
					}
				}
				for childKey, child := range value {
					visit(child, childKey)
				}
			case []any:
				for _, child := range value {
					visit(child, "")
				}
			}
		}
		visit(data, "")
	}
	files, err = filepath.Glob(filepath.Join(root, "discovery/sdext/discoverer/*/metadata.yaml"))
	require.NoError(t, err)
	for _, path := range files {
		bs, err := os.ReadFile(path)
		require.NoError(t, err)
		var data any
		require.NoError(t, yaml.Unmarshal(bs, &data))
		var visit func(any)
		visit = func(value any) {
			switch value := value.(type) {
			case map[any]any:
				if config, ok := value["config"].(string); ok && strings.Contains(config, "config_template:") {
					var parsed any
					name, _ := value["name"].(string)
					require.NoError(t, yaml.Unmarshal([]byte(config), &parsed), path+"/"+name)
					addRules(filepath.Base(filepath.Dir(path))+"/"+name, parsed)
				}
				for _, child := range value {
					visit(child)
				}
			case []any:
				for _, child := range value {
					visit(child)
				}
			}
		}
		visit(data)
	}
	require.NotEmpty(t, sources)
	return sources
}

type discoverySerializationTarget struct {
	model.Base
	Name, Address, IPAddress, Namespace, ContName, Comm, Cmdline, Image, Command string
	Port, PrivatePort                                                            string
	SysInfo                                                                      struct{ Name string }
	Credential                                                                   struct{ Version, Community, UserName, SecurityLevel, AuthProtocol, AuthPassphrase, PrivacyProtocol, PrivacyPassphrase string }
	Annotations, Labels                                                          map[string]any
	Item                                                                         any
	tuid                                                                         string
}

func (t discoverySerializationTarget) TUID() string { return t.tuid }
func (t discoverySerializationTarget) Hash() uint64 { return 1 }

func serializationTarget(value, version, body string) *discoverySerializationTarget {
	tgt := &discoverySerializationTarget{
		Name:        value,
		Address:     value,
		IPAddress:   value,
		Namespace:   value,
		ContName:    value,
		Comm:        value,
		Cmdline:     value,
		Image:       "elasticsearch",
		Command:     "--api-key=" + value,
		Port:        "65534",
		PrivatePort: "65534",
		tuid:        value,
		Annotations: map[string]any{"prometheus.io/path": value},
		Labels:      map[string]any{},
	}
	tgt.SysInfo.Name = value
	tgt.Credential.Version = version
	tgt.Credential.Community, tgt.Credential.UserName, tgt.Credential.SecurityLevel = value, value, value
	tgt.Credential.AuthProtocol, tgt.Credential.AuthPassphrase = value, value
	tgt.Credential.PrivacyProtocol, tgt.Credential.PrivacyPassphrase = value, value
	if strings.Contains(body, ".Item.") || strings.HasPrefix(strings.TrimSpace(body), "{{ .Item |") ||
		strings.Contains(body, "{{ job .Item }}") {
		tgt.Item = map[string]any{"name": value, "url": value, "metrics_url": value, "health_url": value, "module": "httpcheck"}
	} else {
		tgt.Item = value
	}
	return tgt
}

func renderDiscoverySource(t *testing.T, source discoveryTemplateSource, target model.Target) []confgroup.Config {
	t.Helper()
	svr, err := newServiceEngine([]ServiceRuleConfig{{ID: source.id, Match: "{{ true }}", ConfigTemplate: source.body}})
	require.NoError(t, err)
	jobs := svr.compose(target)
	require.NotEmpty(t, jobs, source.name)
	return jobs
}

func replaceDiscoveryMarker(value any, old, replacement string) any {
	switch value := value.(type) {
	case string:
		return strings.ReplaceAll(value, old, replacement)
	case []confgroup.Config:
		out := make([]confgroup.Config, len(value))
		for i, cfg := range value {
			out[i] = replaceDiscoveryMarker(cfg, old, replacement).(confgroup.Config)
		}
		return out
	case confgroup.Config:
		out := confgroup.Config{}
		for k, v := range value {
			out[k] = replaceDiscoveryMarker(v, old, replacement)
		}
		return out
	case map[any]any:
		out := map[any]any{}
		for k, v := range value {
			out[k] = replaceDiscoveryMarker(v, old, replacement)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, v := range value {
			out[i] = replaceDiscoveryMarker(v, old, replacement)
		}
		return out
	default:
		return value
	}
}

func TestStockSNMPStringFieldsAndFallback(t *testing.T) {
	var source discoveryTemplateSource
	for _, candidate := range discoveryTemplateSources(t) {
		if candidate.name == "snmp.conf/snmp" {
			source = candidate
			break
		}
	}
	require.NotEmpty(t, source.body)
	for _, version := range []string{"1", "2", "2c", "3"} {
		for _, name := range []string{"device", ""} {
			t.Run(version+"/"+name, func(t *testing.T) {
				target := serializationTarget("", version, source.body)
				target.SysInfo.Name, target.IPAddress = name, "2001:db8::1"
				job := renderDiscoverySource(t, source, target)
				wantName := "ip-2001:db8::1"
				if name != "" {
					wantName = name + "-" + wantName
				}
				want := confgroup.Config{
					"module":   "snmp",
					"name":     wantName,
					"hostname": "2001:db8::1",
					"options":  map[any]any{"version": version},
				}
				if version != "3" {
					want["community"] = ""
				} else {
					want["user"] = map[any]any{"name": "", "level": "", "auth_proto": "", "auth_key": "", "priv_proto": "", "priv_key": ""}
				}
				assert.Equal(t, []confgroup.Config{want}, job)
			})
		}
	}
}

func TestStockDiscoveryConditionalFields(t *testing.T) {
	sources := map[string]discoveryTemplateSource{}
	for _, source := range discoveryTemplateSources(t) {
		sources[source.name] = source
	}
	for name, test := range map[string]struct {
		source               string
		port, cmdline, image string
		want                 []confgroup.Config
	}{
		"IPv6 memcached": {source: "net_listeners.conf/memcached", want: []confgroup.Config{{"module": "memcached", "name": "local", "address": "[::1]:11211"}}},
		"nginx job order": {source: "docker.conf/nginx", want: []confgroup.Config{
			{"module": "nginx", "name": "docker_on", "url": "http://[::1]:11211/stub_status"},
			{"module": "nginx", "name": "docker_on", "url": "http://[::1]:11211/basic_status"},
			{"module": "nginx", "name": "docker_on", "url": "http://[::1]:11211/nginx_status"},
			{"module": "nginx", "name": "docker_on", "url": "http://[::1]:11211/status"},
		}},
		"OpenSearch fields":    {source: "docker.conf/elasticsearch", image: "opensearch", want: []confgroup.Config{{"module": "elasticsearch", "name": "docker_on", "url": "https://[::1]:11211", "tls_skip_verify": true, "username": "admin", "password": "admin"}}},
		"Elasticsearch fields": {source: "docker.conf/elasticsearch", image: "elasticsearch", want: []confgroup.Config{{"module": "elasticsearch", "name": "docker_on", "url": "http://[::1]:11211"}}},
		"plain kubelet":        {source: "net_listeners.conf/k8s_kubelet", port: "10255", want: []confgroup.Config{{"module": "k8s_kubelet", "name": "local", "url": "http://[::1]:11211/metrics"}}},
		"TLS kubelet":          {source: "net_listeners.conf/k8s_kubelet", port: "10250", want: []confgroup.Config{{"module": "k8s_kubelet", "name": "local", "url": "https://[::1]:11211/metrics", "tls_skip_verify": true}}},
		"numeric port":         {source: "net_listeners.conf/freeradius", port: "18121", want: []confgroup.Config{{"module": "freeradius", "name": "local", "address": "::1", "port": 18121, "secret": "adminsecret"}}},
		"Yugabyte role":        {source: "net_listeners.conf/yugabytedb", port: "12000", want: []confgroup.Config{{"module": "yugabytedb", "name": "local_ycql", "url": "http://[::1]:11211/prometheus-metrics"}}},
		"pushgateway selector": {source: "net_listeners.conf/exporter", port: "9091", want: []confgroup.Config{{"module": "prometheus", "name": "pushgateway_local", "url": "http://[::1]:11211/metrics", "expected_prefix": "pushgateway_", "selector": map[any]any{"allow": []any{"pushgateway_*"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			source, ok := sources[test.source]
			require.True(t, ok)
			target := serializationTarget("on", "2c", source.body)
			target.Address, target.IPAddress = "[::1]:11211", "::1"
			target.Port, target.PrivatePort, target.Cmdline, target.Image = test.port, test.port, test.cmdline, test.image
			assert.Equal(t, test.want, renderDiscoverySource(t, source, target))
		})
	}
}
