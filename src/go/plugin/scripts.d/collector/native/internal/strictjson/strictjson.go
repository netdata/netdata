// SPDX-License-Identifier: GPL-3.0-or-later

// Package strictjson checks field spelling, duplicate keys and nulls before
// encoding/json can coerce them: encoding/json matches field names
// case-insensitively, keeps the last duplicate key and decodes null as a zero
// value. Value types are left to the decoding target.
//
// Errors are fixed text and never contain document values.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"unicode/utf8"
)

// Shape describes the values permitted at one document position. A shape
// restricts object keys and null only; arrays are transparent, so each element
// must match the array's shape. Shapes are immutable and may be shared.
type Shape struct {
	nullable bool
	fields   Fields // permitted object keys
	values   *Shape // when set, any object key is permitted with this value shape
}

// Fields maps permitted object keys to the shapes of their values.
type Fields map[string]*Shape

var (
	scalar         = &Shape{}
	nullableScalar = &Shape{
		nullable: true,
	}
)

// Scalar permits a non-null value without object keys.
func Scalar() *Shape { return scalar }

// Map permits a non-null value with arbitrary object keys whose values match values.
func Map(values *Shape) *Shape {
	return &Shape{
		values: values,
	}
}

// Any permits arbitrary data, including null. Duplicate keys are still rejected.
func Any() *Shape {
	s := &Shape{
		nullable: true,
	}
	s.values = s
	return s
}

// Object permits a non-null value whose object keys are either nested fields or
// scalar fields.
func Object(nested Fields, scalars ...string) *Shape {
	return object(nested, scalar, scalars)
}

// Optional is Object for metadata in which null selects a default: it permits
// null for the value itself and for each scalar field.
func Optional(nested Fields, scalars ...string) *Shape {
	s := object(nested, nullableScalar, scalars)
	s.nullable = true
	return s
}

func object(nested Fields, value *Shape, scalars []string) *Shape {
	fields := make(Fields, len(nested)+len(scalars))
	maps.Copy(fields, nested)
	for _, name := range scalars {
		fields[name] = value
	}
	return &Shape{
		fields: fields,
	}
}

var (
	errInvalidUTF8    = errors.New("JSON must be valid UTF-8")
	errInvalidJSON    = errors.New("expected one complete JSON value")
	errUnexpectedNull = errors.New("unexpected null")
	errUnknownField   = errors.New("unknown field")
	errDuplicateKey   = errors.New("duplicate JSON key")
	errTargetSchema   = errors.New("JSON does not match the expected schema")
)

// validate checks that data is exactly one UTF-8 JSON value matching shape.
func validate(data []byte, shape *Shape) error {
	if !utf8.Valid(data) {
		return errInvalidUTF8
	}
	// json.Valid bounds nesting and rejects incomplete or trailing input before the recursive walk.
	if !json.Valid(data) {
		return errInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Float parse errors would echo the raw number.
	return shape.check(decoder)
}

// Decode validates data against shape and then decodes it into target,
// rejecting fields that target does not declare.
func Decode(data []byte, shape *Shape, target any) error {
	return decode(data, shape, target, false)
}

// DecodeUseNumber is Decode that keeps numbers in untyped values as json.Number.
func DecodeUseNumber(data []byte, shape *Shape, target any) error {
	return decode(data, shape, target, true)
}

func decode(data []byte, shape *Shape, target any, useNumber bool) error {
	if err := validate(data, shape); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if useNumber {
		decoder.UseNumber()
	}
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return errTargetSchema
	}
	return nil
}

func (s *Shape) check(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case nil:
		if !s.nullable {
			return errUnexpectedNull
		}
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key := token.(string)
			child := s.field(key)
			if child == nil {
				return errUnknownField
			}
			if seen[key] {
				return errDuplicateKey
			}
			seen[key] = true
			if err := child.check(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err := s.check(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

func (s *Shape) field(key string) *Shape {
	if s.values != nil {
		return s.values
	}
	return s.fields[key]
}
