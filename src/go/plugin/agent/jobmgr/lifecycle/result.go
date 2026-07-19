// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
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
	deferredBytes := payloadBytes
	if deferredBytes > 0 {
		deferredBytes++
	}
	if deferredBytes > FunctionPayloadBytes {
		return fmt.Errorf("%w: deferred payload is %d bytes", ErrFunctionResultTooLarge, deferredBytes)
	}
	return nil
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
	valueInt64
	valueUint64
	valueFloat64
	valueString
	valueArray
	valueObject
)

type Value struct {
	kind         valueKind
	bool         bool
	int64        int64
	uint64       uint64
	float        float64
	string       string
	repeated     bool
	repeatByte   byte
	repeatLength int
	array        []Value
	object       []ObjectField
	charge       int64
}

type ObjectField struct {
	Key   string
	Value Value
}

func NullValue() Value { return Value{kind: valueNull, charge: functionValueNodeCharge} }
func BoolValue(value bool) Value {
	return Value{kind: valueBool, bool: value, charge: functionValueNodeCharge}
}
func Int64Value(value int64) Value {
	return Value{kind: valueInt64, int64: value, charge: functionValueNodeCharge}
}
func Uint64Value(value uint64) Value {
	return Value{kind: valueUint64, uint64: value, charge: functionValueNodeCharge}
}
func FiniteFloat64Value(value float64) (Value, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Value{}, errors.New("jobmgr Function value: non-finite float")
	}
	return Value{kind: valueFloat64, float: value, charge: functionValueNodeCharge}, nil
}

func StringValue(value string) (Value, error) {
	if !utf8.ValidString(value) {
		return Value{}, errors.New("jobmgr Function value: invalid UTF-8 string")
	}
	charge, err := checkedCharge(functionValueNodeCharge, int64(len(value)))
	if err != nil {
		return Value{}, err
	}
	return Value{kind: valueString, string: strings.Clone(value), charge: charge}, nil
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
	return Value{kind: valueArray, array: append([]Value(nil), values...), charge: charge}, nil
}

func ObjectValue(fields ...ObjectField) (Value, error) {
	owned := append([]ObjectField(nil), fields...)
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
	sort.Slice(owned, func(i, j int) bool { return owned[i].Key < owned[j].Key })
	for index := 1; index < len(owned); index++ {
		if owned[index-1].Key == owned[index].Key {
			return Value{}, errors.New("jobmgr Function value: duplicate object key")
		}
	}
	return Value{kind: valueObject, object: owned, charge: charge}, nil
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
		return strconv.AppendBool(dst, value.bool), nil
	case valueInt64:
		return strconv.AppendInt(dst, value.int64, 10), nil
	case valueUint64:
		return strconv.AppendUint(dst, value.uint64, 10), nil
	case valueFloat64:
		return strconv.AppendFloat(dst, value.float, 'g', -1, 64), nil
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
		return appendJSONString(dst, value.string), nil
	case valueArray:
		dst = append(dst, '[')
		for index, member := range value.array {
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
		for index, field := range value.object {
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
		if value.bool {
			return 4, nil
		}
		return 5, nil
	case valueInt64:
		return len(strconv.FormatInt(value.int64, 10)), nil
	case valueUint64:
		return len(strconv.FormatUint(value.uint64, 10)), nil
	case valueFloat64:
		return len(strconv.FormatFloat(value.float, 'g', -1, 64)), nil
	case valueString:
		if value.repeated {
			return checkedResultSize(value.repeatLength, 2)
		}
		return jsonStringSize(value.string), nil
	case valueArray:
		total := 2
		for index, member := range value.array {
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
		for index, field := range value.object {
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
	total := 0
	maximum := int(^uint(0) >> 1)
	for _, value := range values {
		if value < 0 || total > maximum-value {
			return 0, fmt.Errorf("%w: result size overflow", ErrFunctionResultTooLarge)
		}
		total += value
	}
	return total, nil
}

func checkedCharge(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, fmt.Errorf("%w: plan charge overflow", ErrFunctionResultTooLarge)
		}
		total += value
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

type ReviewedBodySchema uint8

const (
	ReviewedPerformanceJSON ReviewedBodySchema = iota + 1
	ReviewedDynCfgJSON
	ReviewedDynCfgYAML
	ReviewedSNMPTrapJSON
)

type CompleteRawEnvelope struct{ resultPlan }

func NewCompleteRawValueEnvelope(status int, schema ReviewedBodySchema, value Value) (CompleteRawEnvelope, error) {
	if schema != ReviewedPerformanceJSON && schema != ReviewedDynCfgJSON && schema != ReviewedSNMPTrapJSON {
		return CompleteRawEnvelope{}, errors.New("jobmgr Function result: reviewed Value codec is not JSON")
	}
	plan, err := newClosedValuePlan(status, value)
	if err != nil {
		return CompleteRawEnvelope{}, err
	}
	return CompleteRawEnvelope{plan}, nil
}

func NewCompleteRawEnvelope(status int, schema ReviewedBodySchema, body []byte) (CompleteRawEnvelope, error) {
	if status < 100 || status > 599 || !utf8.Valid(body) {
		return CompleteRawEnvelope{}, errors.New("jobmgr Function result: invalid reviewed raw envelope")
	}
	contentType := ""
	switch schema {
	case ReviewedPerformanceJSON, ReviewedDynCfgJSON, ReviewedSNMPTrapJSON:
		if !json.Valid(body) {
			return CompleteRawEnvelope{}, errors.New("jobmgr Function result: invalid reviewed JSON")
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, body); err != nil || !bytes.Equal(compact.Bytes(), body) {
			return CompleteRawEnvelope{}, errors.New("jobmgr Function result: reviewed JSON is not one compact value")
		}
		contentType = "application/json"
	case ReviewedDynCfgYAML:
		if err := validateReviewedYAML(body); err != nil {
			return CompleteRawEnvelope{}, err
		}
		contentType = "application/yaml"
	default:
		return CompleteRawEnvelope{}, errors.New("jobmgr Function result: unknown reviewed body schema")
	}
	if err := validateResultPlanSize(status, contentType, len(body)); err != nil {
		return CompleteRawEnvelope{}, err
	}
	sealed, err := newOwnedSealedResult(status, contentType, bytes.Clone(body))
	if err != nil {
		return CompleteRawEnvelope{}, err
	}
	return CompleteRawEnvelope{resultPlan{sealed: sealed}}, nil
}

func (cre CompleteRawEnvelope) functionResultPlan() resultPlan { return cre.resultPlan }

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

func validateReviewedYAML(body []byte) error {
	if len(body) == 0 {
		return errors.New("jobmgr Function result: empty reviewed YAML")
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "FUNCTION_RESULT_END" {
			return errors.New("jobmgr Function result: unsafe reviewed YAML")
		}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return errors.New("jobmgr Function result: invalid reviewed YAML")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("jobmgr Function result: reviewed YAML must contain one document")
	}
	if err := validateReviewedYAMLNode(&document, 0); err != nil {
		return err
	}
	return nil
}

func validateReviewedYAMLNode(node *yaml.Node, depth int) error {
	if node == nil || depth > MaximumFunctionValueDepth || node.Kind == yaml.AliasNode || node.Anchor != "" || node.Style&yaml.TaggedStyle != 0 {
		return errors.New("jobmgr Function result: unsafe reviewed YAML graph")
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return errors.New("jobmgr Function result: invalid reviewed YAML document")
		}
		return validateReviewedYAMLNode(node.Content[0], depth)
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return errors.New("jobmgr Function result: invalid reviewed YAML mapping")
		}
		keys := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Value == "<<" {
				return errors.New("jobmgr Function result: unsafe reviewed YAML key")
			}
			if _, ok := keys[key.Value]; ok {
				return errors.New("jobmgr Function result: duplicate reviewed YAML key")
			}
			keys[key.Value] = struct{}{}
			if err := validateReviewedYAMLNode(key, depth+1); err != nil {
				return err
			}
			if err := validateReviewedYAMLNode(node.Content[index+1], depth+1); err != nil {
				return err
			}
		}
		return nil
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := validateReviewedYAMLNode(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	case yaml.ScalarNode:
		return nil
	default:
		return errors.New("jobmgr Function result: unknown reviewed YAML node")
	}
}
