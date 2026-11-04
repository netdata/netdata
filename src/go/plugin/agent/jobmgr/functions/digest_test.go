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

	require.EqualValues(t, "1430b84e06cf7c778b7afff945c529a185c8ef2cb6d8f620662bdb194291cf8e", controller)
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
