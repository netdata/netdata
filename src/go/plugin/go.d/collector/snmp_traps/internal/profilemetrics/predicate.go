// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func profileMetricPredicatesMatch(preds []profileMetricPredicate, entry *TrapEntry, td *TrapDef) bool {
	for _, pred := range preds {
		if !profileMetricPredicateMatches(pred, entry, td) {
			return false
		}
	}
	return true
}

func profileMetricPredicateMatches(pred profileMetricPredicate, entry *TrapEntry, td *TrapDef) bool {
	present, value, vb := profileMetricPredicateValue(pred, entry, td)
	result := profileMetricPredicateResult(pred, present, value, vb)
	if pred.Not && present {
		return !result
	}
	return result
}

func profileMetricPredicateValue(pred profileMetricPredicate, entry *TrapEntry, td *TrapDef) (bool, VarbindValue, *VarbindDef) {
	if pred.Field != "" {
		return profileMetricSyntheticFieldValue(pred.Field, entry)
	}
	vb := trapMetricVarbindByName(td, pred.Varbind)
	if vb == nil {
		return false, VarbindValue{}, nil
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	return ok, v, vb
}

func profileMetricSyntheticFieldValue(field string, entry *TrapEntry) (bool, VarbindValue, *VarbindDef) {
	var value string
	switch field {
	case "category":
		value = string(entry.Category)
	case "severity":
		value = string(entry.Severity)
	case "trap_name":
		value = entry.TrapName
	case "trap_oid":
		value = entry.TrapOID
	default:
		return false, VarbindValue{}, nil
	}
	if value == "" {
		return false, VarbindValue{}, nil
	}
	return true, VarbindValue{Value: value}, nil
}

func profileMetricPredicateResult(pred profileMetricPredicate, present bool, value VarbindValue, vb *VarbindDef) bool {
	if pred.Absent != nil {
		return *pred.Absent == !present
	}
	if pred.Exists != nil {
		return *pred.Exists == present
	}
	if !present {
		return false
	}
	if pred.Equals != nil {
		return profileMetricValueEquals(value, vb, pred.Equals)
	}
	if len(pred.In) > 0 {
		return slices.ContainsFunc(pred.In, func(want any) bool {
			return profileMetricValueEquals(value, vb, want)
		})
	}
	if pred.GreaterThan != nil || pred.LessThan != nil || len(pred.Range) > 0 {
		actual, ok := profileMetricFloatValue(value.Value)
		if !ok {
			return false
		}
		if pred.GreaterThan != nil {
			want, ok := profileMetricFloatValue(pred.GreaterThan)
			if !ok || actual <= want {
				return false
			}
		}
		if pred.LessThan != nil {
			want, ok := profileMetricFloatValue(pred.LessThan)
			if !ok || actual >= want {
				return false
			}
		}
		if len(pred.Range) > 0 {
			low, okLow := profileMetricFloatValue(pred.Range[0])
			high, okHigh := profileMetricFloatValue(pred.Range[1])
			if !okLow || !okHigh || actual < low || actual > high {
				return false
			}
		}
		return true
	}
	return false
}

func profileMetricValueEquals(value VarbindValue, vb *VarbindDef, want any) bool {
	actual := model.VarbindRawValue(value)
	if vb != nil && len(vb.Enum) > 0 {
		if label := catalog.ResolveEnum(vb, value.Value); label != "" && label == fmt.Sprintf("%v", want) {
			return true
		}
	}
	return actual == fmt.Sprintf("%v", want)
}

func profileMetricFloatValue(value any) (float64, bool) {
	f, ok := profileMetricRawFloatValue(value)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func profileMetricRawFloatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func parseProfileMetricStateTTL(value string) (time.Duration, error) {
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return ttl, nil
}

type profileMetricValueStatus int

const (
	profileMetricValueOK profileMetricValueStatus = iota
	profileMetricValueMissing
	profileMetricValueInvalid
)

func profileMetricNumericVarbindValue(entry *TrapEntry, vb *VarbindDef) (float64, profileMetricValueStatus) {
	if vb == nil {
		return 0, profileMetricValueInvalid
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	if !ok {
		return 0, profileMetricValueMissing
	}
	value, ok := profileMetricFloatValue(v.Value)
	if !ok {
		return 0, profileMetricValueInvalid
	}
	if strings.EqualFold(strings.TrimSpace(vb.Type), "timeticks") {
		value /= 100
	}
	return value, profileMetricValueOK
}
