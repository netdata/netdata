// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type ValueKind uint8

const (
	ValueNull ValueKind = iota
	ValueString
	ValueInt64
	ValueUint64
	ValueFloat64
	ValueBool
	ValueBytes
)

type CanonicalValue struct {
	Kind    ValueKind
	String  string
	Int64   int64
	Uint64  uint64
	Float64 float64
	Bool    bool
	Bytes   []byte
}

func CanonicalizeValue(value any) CanonicalValue {
	switch value := value.(type) {
	case nil:
		return CanonicalValue{Kind: ValueNull}
	case string:
		return CanonicalValue{Kind: ValueString, String: value}
	case int64:
		return CanonicalValue{Kind: ValueInt64, Int64: value}
	case uint64:
		return CanonicalValue{Kind: ValueUint64, Uint64: value}
	case float64:
		return CanonicalValue{Kind: ValueFloat64, Float64: value}
	case bool:
		return CanonicalValue{Kind: ValueBool, Bool: value}
	case []byte:
		return CanonicalValue{Kind: ValueBytes, Bytes: value}
	default:
		return CanonicalValue{Kind: ValueString, String: fmt.Sprint(value)}
	}
}

func SortedLabelKeys(dst []string, labels map[string]string) []string {
	dst = dst[:0]
	for key := range labels {
		dst = append(dst, key)
	}
	sort.Strings(dst)
	return dst
}

type ProjectedVarbind struct {
	Key   string
	OID   string
	Type  model.ASN1Type
	Value CanonicalValue
	Enum  string
}

type VarbindProjector struct {
	seen map[string]int
}

func (p *VarbindProjector) Reset(capacity int) {
	if p.seen == nil {
		p.seen = make(map[string]int, capacity)
		return
	}
	for key := range p.seen {
		delete(p.seen, key)
	}
}

func (p *VarbindProjector) Reserve(key string) {
	if key != "" {
		p.seen[key]++
	}
}

func (p *VarbindProjector) Project(vb model.VarbindValue) (ProjectedVarbind, bool) {
	key, ok := VarbindKey(vb)
	if !ok {
		return ProjectedVarbind{}, false
	}
	p.seen[key]++
	key = NumberedVarbindKey(key, p.seen[key])
	return ProjectVarbind(key, vb), true
}

func VarbindKey(vb model.VarbindValue) (string, bool) {
	if model.IsSensitiveVarbind(vb) {
		return "", false
	}
	if vb.Name != "" {
		return vb.Name, true
	}
	if vb.OID != "" {
		return vb.OID, true
	}
	return "", false
}

func NumberedVarbindKey(key string, occurrence int) string {
	if occurrence <= 1 {
		return key
	}
	return fmt.Sprintf("%s#%d", key, occurrence)
}

func ProjectVarbind(key string, vb model.VarbindValue) ProjectedVarbind {
	return ProjectedVarbind{
		Key:   key,
		OID:   vb.OID,
		Type:  vb.Type,
		Value: CanonicalizeValue(vb.Value),
		Enum:  vb.Enum,
	}
}
