// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import "fmt"

const LogServiceURI = "/redfish/v1/Managers/1/LogServices/Events"
const LogEntriesURI = LogServiceURI + "/Entries"

// LogDocuments is a synthetic endpoint with all three standard owners linking
// one shared log. Its entries combine inline members, linked members and pages.
func LogDocuments(count int) map[string]map[string]any {
	const root = "/redfish/v1/"
	docs := map[string]map[string]any{
		root: Resource(root, "ServiceRoot", "Root", map[string]any{"RedfishVersion": "1.20.0",
			"Systems": Link(root + "Systems"), "Managers": Link(root + "Managers"), "Chassis": Link(root + "Chassis")}),
		LogServiceURI: Resource(LogServiceURI, "LogService", "Events", map[string]any{"Entries": Link(LogEntriesURI)}),
	}
	for kind, schema := range map[string]string{"Systems": "ComputerSystem", "Managers": "Manager", "Chassis": "Chassis"} {
		collection := root + kind
		owner := collection + "/1"
		docs[collection] = Collection(collection, schema, owner)
		docs[owner] = Resource(
			owner,
			schema,
			kind+" owner",
			map[string]any{"LogServices": Link(owner + "/LogServices")},
		)
		docs[owner+"/LogServices"] = Collection(owner+"/LogServices", "LogService", LogServiceURI)
	}
	page := Collection(LogEntriesURI, "LogEntry")
	docs[LogEntriesURI] = page
	for i := range count {
		if i == 256 {
			page["Members@odata.nextLink"] = LogEntriesURI + "?$skip=256"
			page = Collection(LogEntriesURI, "LogEntry")
			docs[LogEntriesURI+"?$skip=256"] = page
		}
		uri := fmt.Sprintf("%s/%d", LogEntriesURI, i)
		entry := Resource(uri, "LogEntry", fmt.Sprintf("Event %d", i), map[string]any{
			"Message": fmt.Sprintf("Fixture event %d", i), "Severity": "OK", "EntryType": "Event",
			"EventTimestamp": "2023-11-14T22:13:20Z", "Created": "2023-11-14T22:13:21Z",
		})
		if i%2 == 0 {
			docs[uri] = entry
			page["Members"] = append(page["Members"].([]any), Link(uri))
		} else {
			page["Members"] = append(page["Members"].([]any), entry)
		}
	}
	return docs
}
