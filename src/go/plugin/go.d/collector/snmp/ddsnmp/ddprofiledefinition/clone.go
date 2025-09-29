// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ddprofiledefinition

// cloneable is a generic type for objects that can duplicate themselves.
// It is exclusively used in the form [T cloneable[T]], i.e. a type that
// has a .Clone() that returns a new instance of itself.
type cloneable[T any] interface {
	Clone() T
}

// CloneSlice clones all the objects in a slice into a new slice.
func cloneSlice[Slice ~[]T, T cloneable[T]](s Slice) Slice {
	if s == nil {
		return nil
	}
	result := make(Slice, 0, len(s))
	for _, v := range s {
		result = append(result, v.Clone())
	}
	return result
}

// CloneMap clones a map[K]T for any cloneable type T.
// The map keys are shallow-copied; values are cloned.
func cloneMap[Map ~map[K]T, K comparable, T cloneable[T]](m Map) Map {
	if m == nil {
		return nil
	}
	result := make(Map, len(m))
	for k, v := range m {
		result[k] = v.Clone()
	}
	return result
}
