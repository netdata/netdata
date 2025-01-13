// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/hostinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/gohugoio/hashstructure"
	"gopkg.in/yaml.v2"
)

const (
	keyName        = "name"
	keyModule      = "module"
	keyUpdateEvery = "update_every"
	keyDetectRetry = "autodetection_retry"
	keyPriority    = "priority"
	keyLabels      = "labels"
	keyVnode       = "vnode"

	ikeySource     = "__source__"
	ikeySourceType = "__source_type__"
	ikeyProvider   = "__provider__"
)

const (
	TypeStock      = "stock"
	TypeUser       = "user"
	TypeDiscovered = "discovered"
	TypeDyncfg     = "dyncfg"
)

type Config map[string]any

func (c Config) HashIncludeMap(_ string, k, _ any) (bool, error) {
	s := k.(string)
	return !(strings.HasPrefix(s, "__") || strings.HasSuffix(s, "__")), nil
}

func (c Config) Set(key string, value any) Config { c[key] = value; return c }
func (c Config) Get(key string) any               { return c[key] }

func (c Config) Name() string            { v, _ := c.Get(keyName).(string); return v }
func (c Config) Module() string          { v, _ := c.Get(keyModule).(string); return v }
func (c Config) FullName() string        { return fullName(c.Name(), c.Module()) }
func (c Config) UpdateEvery() int        { v, _ := c.Get(keyUpdateEvery).(int); return v }
func (c Config) AutoDetectionRetry() int { v, _ := c.Get(keyDetectRetry).(int); return v }
func (c Config) Priority() int           { v, _ := c.Get(keyPriority).(int); return v }
func (c Config) Labels() map[any]any     { v, _ := c.Get(keyLabels).(map[any]any); return v }
func (c Config) Hash() uint64            { return calcHash(c) }
func (c Config) Vnode() string           { v, _ := c.Get(keyVnode).(string); return v }

func (c Config) SetName(v string) Config   { return c.Set(keyName, v) }
func (c Config) SetModule(v string) Config { return c.Set(keyModule, v) }

func (c Config) UID() string {
	return fmt.Sprintf("%s_%s_%s_%s_%d", c.SourceType(), c.Provider(), c.Source(), c.FullName(), c.Hash())
}

func (c Config) Source() string                { v, _ := c.Get(ikeySource).(string); return v }
func (c Config) SourceType() string            { v, _ := c.Get(ikeySourceType).(string); return v }
func (c Config) Provider() string              { v, _ := c.Get(ikeyProvider).(string); return v }
func (c Config) SetSource(v string) Config     { return c.Set(ikeySource, v) }
func (c Config) SetSourceType(v string) Config { return c.Set(ikeySourceType, v) }
func (c Config) SetProvider(v string) Config   { return c.Set(ikeyProvider, v) }

func (c Config) SourceTypePriority() int {
	switch c.SourceType() {
	default:
		return 0
	case TypeStock:
		return 2
	case TypeDiscovered:
		return 4
	case TypeUser:
		return 8
	case TypeDyncfg:
		return 16
	}
}

func (c Config) Clone() (Config, error) {
	type plain Config
	bytes, err := yaml.Marshal((plain)(c))
	if err != nil {
		return nil, err
	}
	var newConfig Config
	if err := yaml.Unmarshal(bytes, &newConfig); err != nil {
		return nil, err
	}
	return newConfig, nil
}

func (c Config) ApplyDefaults(def Default) {
	if c.UpdateEvery() <= 0 {
		v := firstPositive(def.UpdateEvery, module.UpdateEvery)
		c.Set("update_every", v)
	}
	if c.AutoDetectionRetry() <= 0 {
		v := firstPositive(def.AutoDetectionRetry, module.AutoDetectionRetry)
		c.Set("autodetection_retry", v)
	}
	if c.Priority() <= 0 {
		v := firstPositive(def.Priority, module.Priority)
		c.Set("priority", v)
	}
	if c.UpdateEvery() < def.MinUpdateEvery && def.MinUpdateEvery > 0 {
		c.Set("update_every", def.MinUpdateEvery)
	}
	if c.Name() == "" {
		c.Set("name", c.Module())
	} else {
		c.Set("name", cleanName(jobNameResolveHostname(c.Name())))
	}

	if v, ok := c.Get("url").(string); ok {
		c.Set("url", urlResolveHostname(v))
	}
}

var reInvalidCharacters = regexp.MustCompile(`\s+|\.+|:+`)

func cleanName(name string) string {
	return reInvalidCharacters.ReplaceAllString(name, "_")
}

func fullName(name, module string) string {
	if name == module {
		return name
	}
	return module + "_" + name
}

func calcHash(obj any) uint64 {
	hash, _ := hashstructure.Hash(obj, nil)
	return hash
}

func firstPositive(value int, others ...int) int {
	if value > 0 || len(others) == 0 {
		return value
	}
	return firstPositive(others[0], others[1:]...)
}

func urlResolveHostname(rawURL string) string {
	if hostinfo.Hostname == "" || !strings.Contains(rawURL, "hostname") {
		return rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil || (u.Hostname() != "hostname" && !strings.Contains(u.Hostname(), "hostname.")) {
		return rawURL
	}

	u.Host = strings.Replace(u.Host, "hostname", hostinfo.Hostname, 1)

	return u.String()
}

func jobNameResolveHostname(name string) string {
	if hostinfo.Hostname == "" || !strings.Contains(name, "hostname") {
		return name
	}

	if name != "hostname" && !strings.HasPrefix(name, "hostname.") && !strings.HasPrefix(name, "hostname_") {
		return name
	}

	return strings.Replace(name, "hostname", hostinfo.Hostname, 1)
}
