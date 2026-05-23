// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/gohugoio/hashstructure"
)

const (
	keyName = "name"
	keyKind = "kind"

	ikeySource     = "__source__"
	ikeySourceType = "__source_type__"
)

// Config is the external raw secretstore object shape used by dyncfg/cache layers.
// It carries structural metadata alongside provider payload and excludes __ metadata
// from content hashing.
type Config map[string]any

func (c Config) Set(key string, value any) Config { c[key] = value; return c }

func StoreKey(kind StoreKind, name string) string {
	kind = StoreKind(strings.TrimSpace(string(kind)))
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		return ""
	}
	return string(kind) + ":" + name
}

func ParseStoreKey(key string) (StoreKind, string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("store key is required")
	}

	kindPart, name, ok := strings.Cut(key, ":")
	if !ok {
		return "", "", fmt.Errorf("store key '%s' must be in format 'kind:name'", key)
	}

	kind := StoreKind(strings.TrimSpace(kindPart))
	name = strings.TrimSpace(name)
	if !kind.IsValid() {
		return "", "", fmt.Errorf("invalid store kind '%s'", kind)
	}
	if err := validateStoreName(name); err != nil {
		return "", "", fmt.Errorf("invalid store name '%s': %w", name, err)
	}

	return kind, name, nil
}

func (c Config) Name() string {
	v, _ := c[keyName].(string)
	return v
}

func (c Config) Kind() StoreKind {
	switch v := c[keyKind].(type) {
	case StoreKind:
		return v
	case string:
		return StoreKind(v)
	default:
		return ""
	}
}

func (c Config) Source() string {
	v, _ := c[ikeySource].(string)
	return v
}

func (c Config) SourceType() string {
	v, _ := c[ikeySourceType].(string)
	return v
}

func (c Config) SetName(v string) Config       { return c.Set(keyName, v) }
func (c Config) SetKind(v StoreKind) Config    { return c.Set(keyKind, string(v)) }
func (c Config) SetSource(v string) Config     { return c.Set(ikeySource, v) }
func (c Config) SetSourceType(v string) Config { return c.Set(ikeySourceType, v) }

func (c Config) ExposedKey() string {
	return StoreKey(c.Kind(), c.Name())
}

func (c Config) UID() string {
	if c.Source() == "" || c.ExposedKey() == "" {
		return ""
	}
	return c.Source() + ":" + c.ExposedKey()
}

func (c Config) SourceTypePriority() int {
	switch c.SourceType() {
	case confgroup.TypeDyncfg:
		return 16
	case confgroup.TypeUser:
		return 8
	case confgroup.TypeStock:
		return 2
	default:
		return 0
	}
}

func (c Config) HashIncludeMap(_ string, k, _ any) (bool, error) {
	s := k.(string)
	return !strings.HasPrefix(s, "__") && !strings.HasSuffix(s, "__"), nil
}

func (c Config) Hash() uint64 {
	hash, _ := hashstructure.Hash(c, nil)
	return hash
}

func (c Config) Validate() error {
	if c == nil {
		return fmt.Errorf("store config is nil")
	}

	name := c.Name()
	if name == "" {
		return fmt.Errorf("store name is required")
	}
	if err := validateStoreName(name); err != nil {
		return fmt.Errorf("invalid store name '%s': %w", name, err)
	}

	kind := c.Kind()
	if !kind.IsValid() {
		return fmt.Errorf("invalid store kind '%s'", kind)
	}

	if c.Source() == "" {
		return fmt.Errorf("store source is required")
	}

	switch c.SourceType() {
	case confgroup.TypeDyncfg, confgroup.TypeUser, confgroup.TypeStock:
		return nil
	default:
		return fmt.Errorf("invalid store source type '%s'", c.SourceType())
	}
}
