// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// ProcessFunctionProvider declares Functions owned by the plugin process,
// independently of collector selection or running jobs. ID is stable across
// run generations and supplies the default public name prefix (ID:method).
// Provider IDs must be unique among process providers, even when public names differ.
// Providers and collector modules may share an ID, but public names must not collide.
//
// Functions runs once per generation under framework containment; the selected
// handler factory runs when declarations are nonempty. They must return fresh declarations and
// a fresh handler. Handler Cleanup owns
// generation-local resources only; injected process stores remain caller-owned.
// Available predicates use the normal process tick even when no collectors run.
// As for collector AgentFunctions, once published a Function stays advertised;
// later false results do not withdraw it or prevent dispatch. The handler owns
// whether retained data is currently readable.
//
// Callbacks must return promptly. Shutdown/restart joins physical callback and
// handler cleanup ownership; noncooperation can require a full process restart.
type ProcessFunctionProvider struct {
	ID         string
	Functions  func() []FunctionConfig
	NewHandler func() MethodHandler
	// Exactly one factory must be set. The context-aware factory receives
	// generation-owned capabilities without changing existing providers.
	NewHandlerWithContext func(ProcessFunctionContext) MethodHandler
}

// ProcessFunctionContext supplies borrowed, read-only generation capabilities.
// Handlers must not retain them beyond Cleanup or close their underlying owners.
type ProcessFunctionContext struct {
	// LookupVnodeGUID resolves a configured name to its canonical GUID, including
	// vnodes without running jobs or acquired metadata. Unknown names return false.
	// It is safe for concurrent calls and reflects committed configuration updates.
	// It is nil when constructing a handler without production composition.
	LookupVnodeGUID func(name string) (guid string, ok bool)
}
