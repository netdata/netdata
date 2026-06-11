// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import "fmt"

// MethodFunctionName returns the primary public Function name for a module method.
func MethodFunctionName(moduleName string, method MethodConfig) string {
	if method.FunctionName != "" {
		return method.FunctionName
	}
	return fmt.Sprintf("%s:%s", moduleName, method.ID)
}

// MethodFunctionNames returns the primary public Function name plus aliases.
func MethodFunctionNames(moduleName string, method MethodConfig) []string {
	funcName := MethodFunctionName(moduleName, method)
	funcNames := []string{funcName}
	seen := map[string]struct{}{funcName: {}}

	for _, alias := range method.Aliases {
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		funcNames = append(funcNames, alias)
	}
	return funcNames
}
