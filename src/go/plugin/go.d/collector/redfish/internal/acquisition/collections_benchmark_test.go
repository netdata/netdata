// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

func BenchmarkCollectionPage(b *testing.B) {
	root, origin, err := NormalizeServiceRoot("https://bmc.example/redfish/v1/")
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{
		root:   root,
		origin: origin,
	}
	target, err := url.Parse("https://bmc.example/redfish/v1/Sensors")
	if err != nil {
		b.Fatal(err)
	}
	members := make([]map[string]string, 100)
	for i := range members {
		members[i] = map[string]string{"@odata.id": fmt.Sprintf("/redfish/v1/Sensors/%d", i)}
	}
	body, err := json.Marshal(map[string]any{
		"@odata.id": "/redfish/v1/Sensors", "@odata.type": "#SensorCollection.SensorCollection",
		"Members@odata.count": len(members), "Members": members,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		got, complete, err := client.fetchCollectionMemberPages(
			context.Background(),
			target,
			&responseData{
				url:  target,
				body: body,
			},
			nil,
		)
		if err != nil || !complete || len(got) != len(members) {
			b.Fatalf("members=%d complete=%v err=%v", len(got), complete, err)
		}
	}
}
