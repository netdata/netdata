// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

type graphSlice struct {
	ParentKey string
	Path      string
	ChildKind string
}

type graphMembershipSnapshot struct {
	ParentKey    string
	Relationship graphRelationship
	Members      []graphMemberIdentity
}

func (g *resourceGraph) recordMembership(parent *graphNode, rel graphRelationship) {
	g.Slices = append(g.Slices, graphSlice{
		ParentKey: parent.Key,
		Path:      rel.Path,
		ChildKind: rel.ChildKind,
	})
}

func graphMembershipKey(parentKey string, rel graphRelationship) string {
	return parentKey + "\x00" + rel.Path + "\x00" + rel.ChildKind
}

func (c *Client) finalizeGraphMembership(graph *resourceGraph, traversalFinished bool) error {
	if graph == nil {
		return nil
	}
	observed := make(map[string]bool, len(graph.Slices))
	for _, slice := range graph.Slices {
		observed[slice.ParentKey+"\x00"+slice.Path+"\x00"+slice.ChildKind] = true
	}
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	if graph.Complete {
		for key := range c.graphMembership {
			if !observed[key] {
				delete(c.graphMembership, key)
			}
		}
		return nil
	}
	// Unvisited branches retain identities only, so a failed read cannot replay measurements.
	pending := make(map[string][]string)
	for key, snapshot := range c.graphMembership {
		if !observed[key] {
			pending[snapshot.ParentKey] = append(pending[snapshot.ParentKey], key)
		}
	}
	queue := append([]*graphNode(nil), graph.Nodes...)
	for pos := 0; pos < len(queue); pos++ {
		parent := queue[pos]
		keys := pending[parent.Key]
		delete(pending, parent.Key)
		for _, key := range keys {
			snapshot := c.graphMembership[key]
			children := restoreGraphMembers(snapshot.Members)
			for _, child := range children {
				if err := graph.addChild(parent, child, &queue); err != nil {
					return err
				}
			}
			graph.recordMembership(parent, snapshot.Relationship)
		}
	}
	// Base discovery retains unknown roots, and incomplete relationships retain
	// their members. After restoration, unreachable parents were therefore removed
	// authoritatively. An aborted traversal cannot establish that proof.
	if traversalFinished {
		for _, keys := range pending {
			for _, key := range keys {
				delete(c.graphMembership, key)
			}
		}
	}
	return nil
}

func (c *Client) reconcileGraphMembership(
	parent *graphNode,
	rel graphRelationship,
	current []*graphNode,
	complete bool,
) []*graphNode {
	key := graphMembershipKey(parent.Key, rel)
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	if c.graphMembership == nil {
		c.graphMembership = make(map[string]graphMembershipSnapshot)
	}
	if !complete {
		seen := make(map[string]*graphNode, len(current))
		for _, node := range current {
			seen[node.Key] = node
		}
		for _, retained := range restoreGraphMembers(c.graphMembership[key].Members) {
			if node := seen[retained.Key]; node != nil {
				if node.AcquisitionState != "readable" {
					node.Doc = retained.Doc
				}
			} else {
				current = append(current, retained)
			}
		}
	}
	if len(current) == 0 {
		delete(c.graphMembership, key)
	} else {
		c.graphMembership[key] = graphMembershipSnapshot{
			ParentKey:    parent.Key,
			Relationship: rel,
			Members:      snapshotGraphMembers(current),
		}
	}
	return current
}

type graphMemberIdentity struct {
	Kind, URI, Locator, Key                  string
	IdentityQuality, SourcePath, SourceModel string
	Display                                  resourceDisplayIdentity
}

func snapshotGraphMembers(nodes []*graphNode) []graphMemberIdentity {
	result := make([]graphMemberIdentity, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		result = append(result, graphMemberIdentity{
			Kind:            node.Kind,
			URI:             node.URI,
			Locator:         node.Locator,
			Key:             node.Key,
			IdentityQuality: node.IdentityQuality,
			SourcePath:      node.SourcePath,
			SourceModel:     node.SourceModel,
			Display:         snapshotResourceDisplay(node.Doc),
		})
	}
	return result
}

func restoreGraphMembers(members []graphMemberIdentity) []*graphNode {
	result := make([]*graphNode, 0, len(members))
	for _, member := range members {
		result = append(result, &graphNode{
			Kind:             member.Kind,
			URI:              member.URI,
			Locator:          member.Locator,
			Key:              member.Key,
			IdentityQuality:  member.IdentityQuality,
			SourcePath:       member.SourcePath,
			SourceModel:      member.SourceModel,
			Doc:              member.Display.document(),
			AcquisitionState: "unknown",
			Parents:          make(map[string]*graphNode),
		})
	}
	return result
}
