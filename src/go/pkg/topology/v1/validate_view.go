// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import "fmt"

func validateViewFocus(raw any, actorRows int) error {
	view, ok := raw.(map[string]any)
	if !ok {
		return nil // The schema validates the view object's shape.
	}
	rawFocus, exists := view["focus"]
	if !exists {
		return nil
	}
	focus, ok := rawFocus.(map[string]any)
	if !ok {
		return fmt.Errorf("data.view.focus is not an object")
	}
	roots, ok := focus["roots"].([]any)
	if !ok {
		return fmt.Errorf("data.view.focus.roots is not an array")
	}
	seen := make(map[int]bool, len(roots))
	for i, rawRoot := range roots {
		root, ok := integerValue(rawRoot)
		if !ok || root < 0 || root >= actorRows {
			return fmt.Errorf("data.view.focus.roots[%d] is not a valid actor row reference", i)
		}
		if seen[root] {
			return fmt.Errorf("data.view.focus.roots[%d] repeats actor row %d", i, root)
		}
		seen[root] = true
	}
	if rawDepth, exists := focus["depth"]; exists {
		depth, ok := integerValue(rawDepth)
		if !ok || depth < 0 {
			return fmt.Errorf("data.view.focus.depth is not a non-negative integer")
		}
	}
	return nil
}
