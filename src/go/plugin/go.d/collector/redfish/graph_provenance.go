// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

// reconcileSensorExcerptIdentities runs after traversal and retained-identity
// restoration. A DataSourceUri pointer proves an alias only when its containing
// resource is independently known as a Sensor; a pointer alone cannot prove it.
func (c *protocolClient) reconcileSensorExcerptIdentities(graph *resourceGraph) {
	aliases := c.mergeProvenSensorExcerpts(graph)
	if len(aliases) == 0 {
		return
	}
	graph.replaceSensorAliases(aliases)
	c.reconcileSensorMembership(aliases)
}

func (c *protocolClient) mergeProvenSensorExcerpts(graph *resourceGraph) map[string]*graphNode {
	addressable := make(map[string]*graphNode)
	for _, node := range graph.Nodes {
		if node.Kind == "sensor" && node.IdentityQuality == "addressable" {
			addressable[node.Locator] = node
		}
	}
	aliases := make(map[string]*graphNode)
	excerpts := make(map[*graphNode][][]sensorExcerptSource)
	for _, node := range graph.Nodes {
		if node.Kind != "sensor" || node.IdentityQuality != "data_source_uri" ||
			node.SourceModel != "embedded_sensor_excerpt" {
			continue
		}
		target, err := resolveRedfishURI(c.origin, c.root, node.Locator, uriProvenance)
		if err != nil {
			continue
		}
		uri := canonicalResourceURI(target)
		sensor := addressable[uri]
		if sensor == nil || sensor == node {
			continue
		}
		aliases[node.Key] = sensor
		// Keep the pointer in SensorExcerpts.Data, while resource identity belongs to
		// the proven Sensor. Addressable values and source health keep precedence.
		candidate := *node
		candidate.URI, candidate.Locator, candidate.Key = uri, uri, sensor.Key
		if _, exists := excerpts[sensor]; !exists {
			excerpts[sensor] = [][]sensorExcerptSource{sensor.SensorExcerpts}
		}
		excerpts[sensor] = append(excerpts[sensor], node.SensorExcerpts)
		candidate.SensorExcerpts = nil
		sensor.SensorExcerpts = nil
		mergeEquivalentGraphNode(sensor, &candidate)
		for key, parent := range node.Parents {
			sensor.Parents[key] = parent
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	for sensor, sources := range excerpts {
		sensor.SensorExcerpts = mergeSensorExcerptSources(sources...)
	}
	return aliases
}

func (graph *resourceGraph) replaceSensorAliases(aliases map[string]*graphNode) {
	nodes := graph.Nodes[:0]
	for _, node := range graph.Nodes {
		if aliases[node.Key] != nil {
			delete(graph.ByIdentity, node.Kind+"\x00"+node.Locator)
			delete(graph.KeySources, node.Key)
			continue
		}
		for key, parent := range node.Parents {
			if replacement := aliases[parent.Key]; replacement != nil {
				delete(node.Parents, key)
				node.Parents[replacement.Key] = replacement
			}
		}
		nodes = append(nodes, node)
	}
	graph.Nodes = nodes
	for index := range graph.Slices {
		if replacement := aliases[graph.Slices[index].ParentKey]; replacement != nil {
			graph.Slices[index].ParentKey = replacement.Key
		}
	}
}

func (c *protocolClient) reconcileSensorMembership(aliases map[string]*graphNode) {
	// Membership snapshots use the same final identity, so a later failed cycle
	// cannot resurrect an already-proven alias as a second unknown resource.
	c.graphMu.Lock()
	defer c.graphMu.Unlock()
	reconciled := make(map[string]graphMembershipSnapshot, len(c.graphMembership))
	seenBySlice := make(map[string]map[string]bool, len(c.graphMembership))
	for _, snapshot := range c.graphMembership {
		if replacement := aliases[snapshot.ParentKey]; replacement != nil {
			snapshot.ParentKey = replacement.Key
		}
		key := graphMembershipKey(snapshot.ParentKey, snapshot.Relationship)
		previous := reconciled[key]
		seen := seenBySlice[key]
		if seen == nil {
			seen = make(map[string]bool, len(snapshot.Members))
			seenBySlice[key] = seen
		}
		members := previous.Members
		for _, member := range snapshot.Members {
			if replacement := aliases[member.Key]; replacement != nil {
				member = snapshotGraphMembers([]*graphNode{replacement})[0]
			}
			if !seen[member.Key] {
				members = append(members, member)
				seen[member.Key] = true
			}
		}
		snapshot.Members = members
		reconciled[key] = snapshot
	}
	c.graphMembership = reconciled
}
