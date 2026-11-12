// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/require"
)

func TestControllerGroupSignatureLayoutGolden(t *testing.T) {
	controller, err := controllerGroupSignature(
		methodGenerationAgent,
		[]funcapi.FunctionConfig{{
			ID: "method", FunctionName: "module:method", Name: "Method",
			UpdateEvery: 5, Help: "method help", RequireCloud: true,
			Tags: "top", ResponseType: "table", RawRequest: true,
			Aliases: []string{"method-alias"},
		}},
		nil,
	)
	require.NoError(t, err)

	require.EqualValues(t, "102664680e1aca7d56af4733129a78fbbc9b01ec05479f30e784c668153cb385", controller)
}

func TestControllerGroupSignatureManagedMetadata(t *testing.T) {
	base := funcapi.FunctionConfig{
		ID:         "events",
		RawRequest: true,
	}
	before, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{base}, nil)
	require.NoError(t, err)
	for name, change := range map[string]func(*funcapi.FunctionConfig){
		"managed info":    func(method *funcapi.FunctionConfig) { method.ManagedInfo = true },
		"history":         func(method *funcapi.FunctionConfig) { method.HasHistory = true },
		"accepted inputs": func(method *funcapi.FunctionConfig) { method.AcceptedParams = []string{"after"} },
		"commercial":      func(method *funcapi.FunctionConfig) { method.RequireCommercial = true },
	} {
		t.Run(name, func(t *testing.T) {
			method := base
			change(&method)
			after, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
			require.NoError(t, err)
			require.NotEqual(t, before, after, "changed metadata must replace the published generation")
		})
	}
}

func TestControllerGroupSignatureDynamicInfo(t *testing.T) {
	method := funcapi.FunctionConfig{ID: "events"}
	before, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
	require.NoError(t, err)
	method.DynamicInfo = true
	after, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestControllerGroupSignatureAcceptedForms(t *testing.T) {
	for name, change := range map[string]func(*funcapi.ParamConfig){
		"separator": func(param *funcapi.ParamConfig) { param.ValueSeparator = "," },
		"aliases":   func(param *funcapi.ParamConfig) { param.Options[0].Aliases = []string{"alternate"} },
		"alias boundaries": func(param *funcapi.ParamConfig) {
			param.Options[0].Aliases = []string{"a", "bc"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			method := funcapi.FunctionConfig{ID: "events", RequiredParams: []funcapi.ParamConfig{
				{ID: "source", Options: []funcapi.ParamOption{{ID: "canonical", Aliases: []string{"ab", "c"}}}},
			}}
			before, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
			require.NoError(t, err)
			change(&method.RequiredParams[0])
			after, err := controllerGroupSignature(methodGenerationShared, []funcapi.FunctionConfig{method}, nil)
			require.NoError(t, err)
			require.NotEqual(t, before, after)
		})
	}
}
