// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type resource interface {
	source() string
	kind() kubeResourceKind
	value() any
}

type kubeResourceKind uint8

const (
	kubeResourceNode kubeResourceKind = iota + 1
	kubeResourcePod
)

func toNode(i any) (*corev1.Node, error) {
	switch v := i.(type) {
	case *corev1.Node:
		return v, nil
	case resource:
		return toNode(v.value())
	default:
		return nil, fmt.Errorf("unexpected type: %T (expected %T or %T)", v, &corev1.Node{}, resource(nil))
	}
}

func toPod(i any) (*corev1.Pod, error) {
	switch v := i.(type) {
	case *corev1.Pod:
		return v, nil
	case resource:
		return toPod(v.value())
	default:
		return nil, fmt.Errorf("unexpected type: %T (expected %T or %T)", v, &corev1.Pod{}, resource(nil))
	}
}
