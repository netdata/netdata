// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import "fmt"

// FunctionName returns the primary public Function name for a module method.
func FunctionName(moduleName string, method FunctionConfig) string {
	if method.FunctionName != "" {
		return method.FunctionName
	}
	return fmt.Sprintf("%s:%s", moduleName, method.ID)
}

// FunctionNames returns the primary public Function name plus aliases.
func FunctionNames(moduleName string, method FunctionConfig) []string {
	funcName := FunctionName(moduleName, method)
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
