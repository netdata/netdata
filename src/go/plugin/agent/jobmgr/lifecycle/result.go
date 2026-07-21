// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaximumFunctionValueDepth = 128

const (
	functionValueNodeCharge   = int64(64)
	functionObjectFieldCharge = int64(16)
)

type FunctionResult interface {
	Size() int
	Append([]byte) []byte
	functionResultPlan() resultPlan
}

type resultPlan struct {
	sealed SealedResult
}

func (rp resultPlan) Size() int {
	return rp.sealed.payloadBytes
}

func (rp resultPlan) Append(dst []byte) []byte {
	appended, err := rp.sealed.appendPayload(dst)
	if err != nil {
		panic(err)
	}
	return appended
}

func (rp resultPlan) validate() error {
	return rp.sealed.validate()
}

func validateResultPlanSize(status int, contentType string, payloadBytes int) error {
	if status < 100 || status > 599 || contentType == "" || payloadBytes < 0 {
		return errors.New("jobmgr Function result: invalid immutable plan")
	}
	return validateFunctionPayloadSize(payloadBytes)
}

type ErrorResult struct{ resultPlan }

func NewErrorResult(status int, message string) (ErrorResult, error) {
	if status < 400 || status > 599 || !utf8.ValidString(message) {
		return ErrorResult{}, errors.New("jobmgr Function result: invalid typed error")
	}
	payload := make([]byte, 0, len(message)+40)
	payload = append(payload, `{"errorMessage":`...)
	payload = appendJSONString(payload, message)
	payload = append(payload, `,"status":`...)
	payload = strconv.AppendInt(payload, int64(status), 10)
	payload = append(payload, '}')
	sealed, err := newOwnedSealedResult(status, "application/json", payload)
	if err != nil {
		return ErrorResult{}, err
	}
	return ErrorResult{resultPlan{sealed: sealed}}, nil
}

func (er ErrorResult) functionResultPlan() resultPlan { return er.resultPlan }

type valueKind uint8

const (
	valueInvalid valueKind = iota
	valueNull
	valueBool
	valueUint64
	valueFloat64
	valueString
	valueArray
	valueObject
)

type Value struct {
	str          string        // string payload (kind == string)
	arr          []Value       // array elements (kind == array)
	obj          []ObjectField // object fields (kind == object)
	u64          uint64        // unsigned payload (kind == uint64)
	f64          float64       // float payload (kind == float)
	repeatLength int           // length of a synthetic repeated-byte string
	charge       int64         // accounting charge propagated into SealedResult.planBytes
	kind         valueKind     // variant discriminant
	boolean      bool          // bool payload (kind == bool)
	repeated     bool          // value is a synthetic repeated-byte string (bench/large payloads)
	repeatByte   byte          // the repeated byte
}

type ObjectField struct {
	Key   string
	Value Value
}

func NullValue() Value { return Value{kind: valueNull, charge: functionValueNodeCharge} }
func BoolValue(value bool) Value {
	return Value{kind: valueBool, boolean: value, charge: functionValueNodeCharge}
}
func Uint64Value(value uint64) Value {
	return Value{kind: valueUint64, u64: value, charge: functionValueNodeCharge}
}
func FiniteFloat64Value(value float64) (Value, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Value{}, errors.New("jobmgr Function value: non-finite float")
	}
	return Value{kind: valueFloat64, f64: value, charge: functionValueNodeCharge}, nil
}

func StringValue(value string) (Value, error) {
	if !utf8.ValidString(value) {
		return Value{}, errors.New("jobmgr Function value: invalid UTF-8 string")
	}
	charge, err := checkedCharge(functionValueNodeCharge, int64(len(value)))
	if err != nil {
		return Value{}, err
	}
	return Value{kind: valueString, str: strings.Clone(value), charge: charge}, nil
}

func RepeatedStringValue(length int, fill byte) (Value, error) {
	if length < 0 || fill < 'A' || fill > 'Z' {
		return Value{}, errors.New("jobmgr Function value: invalid repeated string")
	}
	return Value{kind: valueString, repeated: true, repeatByte: fill, repeatLength: length, charge: functionValueNodeCharge}, nil
}

func ArrayValue(values ...Value) (Value, error) {
	charge := functionValueNodeCharge
	for _, value := range values {
		if value.kind == valueInvalid {
			return Value{}, errors.New("jobmgr Function value: invalid array member")
		}
		var err error
		charge, err = checkedCharge(charge, value.charge)
		if err != nil {
			return Value{}, err
		}
	}
	return Value{kind: valueArray, arr: slices.Clone(values), charge: charge}, nil
}

func ObjectValue(fields ...ObjectField) (Value, error) {
	owned := slices.Clone(fields)
	charge := functionValueNodeCharge
	for index := range owned {
		if !utf8.ValidString(owned[index].Key) || owned[index].Value.kind == valueInvalid {
			return Value{}, errors.New("jobmgr Function value: invalid object field")
		}
		owned[index].Key = strings.Clone(owned[index].Key)
		var err error
		charge, err = checkedCharge(charge, functionObjectFieldCharge, int64(len(owned[index].Key)), owned[index].Value.charge)
		if err != nil {
			return Value{}, err
		}
	}
	slices.SortFunc(owned, func(a, b ObjectField) int { return cmp.Compare(a.Key, b.Key) })
	for index := 1; index < len(owned); index++ {
		if owned[index-1].Key == owned[index].Key {
			return Value{}, errors.New("jobmgr Function value: duplicate object key")
		}
	}
	return Value{kind: valueObject, obj: owned, charge: charge}, nil
}

type TableResult struct{ resultPlan }

func NewTableResult(status int, table Value) (TableResult, error) {
	plan, err := newClosedValuePlan(status, table)
	if err != nil {
		return TableResult{}, err
	}
	return TableResult{plan}, nil
}

func (tr TableResult) functionResultPlan() resultPlan { return tr.resultPlan }

type TopologyResult struct{ resultPlan }

func NewTopologyResult(status int, topology Value) (TopologyResult, error) {
	plan, err := newClosedValuePlan(status, topology)
	if err != nil {
		return TopologyResult{}, err
	}
	return TopologyResult{plan}, nil
}

func (tr TopologyResult) functionResultPlan() resultPlan { return tr.resultPlan }

func newClosedValuePlan(status int, value Value) (resultPlan, error) {
	if status < 100 || status > 599 || value.kind == valueInvalid {
		return resultPlan{}, errors.New("jobmgr Function result: invalid closed value result")
	}
	size, err := valueJSONSize(value, 0)
	if err != nil {
		return resultPlan{}, err
	}
	if err := validateResultPlanSize(status, "application/json", size); err != nil {
		return resultPlan{}, err
	}
	sealed := SealedResult{
		status: status, contentType: "application/json", payloadKind: sealedPayloadValue,
		value: value, payloadBytes: size, planBytes: value.charge,
	}
	plan := resultPlan{sealed: sealed}
	if err := plan.validate(); err != nil {
		return resultPlan{}, err
	}
	return plan, nil
}

func appendValueJSON(dst []byte, value Value, depth int) ([]byte, error) {
	if depth > MaximumFunctionValueDepth {
		return nil, errors.New("jobmgr Function value: maximum container depth exceeded")
	}
	switch value.kind {
	case valueNull:
		return append(dst, "null"...), nil
	case valueBool:
		return strconv.AppendBool(dst, value.boolean), nil
	case valueUint64:
		return strconv.AppendUint(dst, value.u64, 10), nil
	case valueFloat64:
		return strconv.AppendFloat(dst, value.f64, 'g', -1, 64), nil
	case valueString:
		if value.repeated {
			dst = append(dst, '"')
			dst = slices.Grow(dst, value.repeatLength)
			start := len(dst)
			dst = dst[:start+value.repeatLength]
			for index := start; index < len(dst); index++ {
				dst[index] = value.repeatByte
			}
			return append(dst, '"'), nil
		}
		return appendJSONString(dst, value.str), nil
	case valueArray:
		dst = append(dst, '[')
		for index, member := range value.arr {
			if index > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendValueJSON(dst, member, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	case valueObject:
		dst = append(dst, '{')
		for index, field := range value.obj {
			if index > 0 {
				dst = append(dst, ',')
			}
			dst = appendJSONString(dst, field.Key)
			dst = append(dst, ':')
			var err error
			dst, err = appendValueJSON(dst, field.Value, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, '}'), nil
	default:
		return nil, errors.New("jobmgr Function value: unknown variant")
	}
}

func valueJSONSize(value Value, depth int) (int, error) {
	if depth > MaximumFunctionValueDepth {
		return 0, errors.New("jobmgr Function value: maximum container depth exceeded")
	}
	switch value.kind {
	case valueNull:
		return 4, nil
	case valueBool:
		if value.boolean {
			return 4, nil
		}
		return 5, nil
	case valueUint64:
		return len(strconv.FormatUint(value.u64, 10)), nil
	case valueFloat64:
		return len(strconv.FormatFloat(value.f64, 'g', -1, 64)), nil
	case valueString:
		if value.repeated {
			return checkedResultSize(value.repeatLength, 2)
		}
		return jsonStringSize(value.str), nil
	case valueArray:
		total := 2
		for index, member := range value.arr {
			size, err := valueJSONSize(member, depth+1)
			if err != nil {
				return 0, err
			}
			if index > 0 {
				total, err = checkedResultSize(total, 1)
				if err != nil {
					return 0, err
				}
			}
			total, err = checkedResultSize(total, size)
			if err != nil {
				return 0, err
			}
		}
		return total, nil
	case valueObject:
		total := 2
		for index, field := range value.obj {
			valueSize, err := valueJSONSize(field.Value, depth+1)
			if err != nil {
				return 0, err
			}
			if index > 0 {
				total, err = checkedResultSize(total, 1)
				if err != nil {
					return 0, err
				}
			}
			total, err = checkedResultSize(total, jsonStringSize(field.Key), 1, valueSize)
			if err != nil {
				return 0, err
			}
		}
		return total, nil
	default:
		return 0, errors.New("jobmgr Function value: unknown variant")
	}
}

func checkedResultSize(values ...int) (int, error) {
	total, ok := checkedSum(int(^uint(0)>>1), values...)
	if !ok {
		return 0, fmt.Errorf("%w: result size overflow", ErrFunctionResultTooLarge)
	}
	return total, nil
}

func checkedCharge(values ...int64) (int64, error) {
	total, ok := checkedSum(int64(math.MaxInt64), values...)
	if !ok {
		return 0, fmt.Errorf("%w: plan charge overflow", ErrFunctionResultTooLarge)
	}
	return total, nil
}

func jsonStringSize(value string) int {
	size := 2
	for _, r := range value {
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			size += 2
		case r < 0x20 || r == '<' || r == '>' || r == '&' || r == '\u2028' || r == '\u2029':
			size += 6
		default:
			size += utf8.RuneLen(r)
		}
	}
	return size
}

func appendJSONString(dst []byte, value string) []byte {
	encoded, _ := json.Marshal(value)
	return append(dst, encoded...)
}

func SealFunctionResult(result FunctionResult) (SealedResult, error) {
	if result == nil {
		return SealedResult{}, errors.New("jobmgr Function result: nil result")
	}
	plan := result.functionResultPlan()
	if err := plan.validate(); err != nil || result.Size() != plan.sealed.payloadBytes {
		return SealedResult{}, errors.Join(errors.New("jobmgr Function result: invalid sealed plan"), err)
	}
	return plan.sealed, nil
}
