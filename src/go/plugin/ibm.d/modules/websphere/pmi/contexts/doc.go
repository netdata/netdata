// Package contexts defines metric contexts for the WebSphere PMI module.
// This file triggers go:generate to produce zz_generated_contexts.go from contexts.yaml.
package contexts

//go:generate go run ../../../../metricgen/main.go -module=websphere_pmi -input=contexts.yaml -output=zz_generated_contexts.go -package=contexts
