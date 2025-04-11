// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"encoding/json"
	"github.com/invopop/jsonschema"
)

// ListMap is used to marshall a map into a list (map[string]T to []MapItem[T]) and vice versa.
type ListMap[T any] map[string]T

// MapItem is used for ListMap marshalling/unmarshalling
type MapItem[T any] struct {
	Key   string `json:"key"`
	Value T      `json:"value"`
}

// MarshalJSON marshalls map to list
func (lm ListMap[T]) MarshalJSON() ([]byte, error) {
	var items []MapItem[T]
	for key, value := range lm {
		items = append(items, MapItem[T]{Key: key, Value: value})
	}
	return json.Marshal(items)
}

// UnmarshalJSON unmarshalls list to map
func (lm *ListMap[T]) UnmarshalJSON(data []byte) error {
	var items []MapItem[T]
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	result := make(ListMap[T])
	for _, item := range items {
		result[item.Key] = item.Value
	}
	*lm = result
	return nil
}

// JSONSchema is needed to customize jsonschema to match []MapItem[T] used in json format
func (lm ListMap[T]) JSONSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect([]MapItem[T]{})
	// don't need version because this is a child of a versioned schema.
	schema.Version = ""
	return schema
}
