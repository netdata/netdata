// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"fmt"
	"testing"
	"time"
)

// Projection and view construction should grow linearly with acquired resources.
func BenchmarkProject(b *testing.B) {
	for _, count := range []int{10, 100} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			resources := make([]*Resource, count)
			for i := range resources {
				name := fmt.Sprintf("Temperature %d", i)
				resources[i] = &Resource{
					Kind:             "sensor",
					Key:              name,
					URI:              "/redfish/v1/Sensors/" + fmt.Sprint(i),
					AcquisitionState: "readable",
					Doc: Document{
						Name: name,
					},
					Data: map[string]any{
						"Name":         name,
						"ReadingType":  "Temperature",
						"ReadingUnits": "Cel",
						"Reading":      42.0,
						"Status":       map[string]any{"Health": "OK", "State": "Enabled"},
					},
				}
			}
			p := New("https://bmc.example.test", "benchmark", nil)
			observed := time.Unix(1000, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := p.Project(resources, true, observed); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
