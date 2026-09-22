// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"fmt"
	"testing"
	"time"
)

// Projection and view construction should grow linearly with acquired resources.
func BenchmarkProject(b *testing.B) {
	benchmarkProject(b, false)
}

func BenchmarkProjectThresholds(b *testing.B) {
	benchmarkProject(b, true)
}

func benchmarkProject(b *testing.B, thresholds bool) {
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
				if thresholds {
					resources[i].Data["Thresholds"] = map[string]any{
						"UpperCritical": map[string]any{
							"Reading": 50, "DwellTime": "PT10S",
							"HysteresisDuration": "PT10S", "HysteresisReading": -1,
						},
					}
				}
			}
			p := New("https://bmc.example.test", nil)
			observed := time.Unix(1000, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if thresholds {
					observed = observed.Add(time.Second)
				}
				if _, err := p.Project(resources, true, observed); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Inventory uses a fixed field set per resource, with copied optional numbers.
func BenchmarkProjectInventory(b *testing.B) {
	kinds := []string{"processor", "memory", "drive", "volume", "firmware", "software", "assembly"}
	for _, count := range []int{14, 140} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			resources := make([]*Resource, count)
			for i := range resources {
				kind := kinds[i%len(kinds)]
				name := fmt.Sprintf("%s %d", kind, i)
				data := map[string]any{"Status": map[string]any{"Health": "OK"}}
				switch kind {
				case "processor":
					data["TotalCores"], data["TotalEnabledCores"], data["TotalThreads"] = 32, 24, 64
				case "memory":
					data["CapacityMiB"], data["MemoryDeviceType"] = 32768, "DDR5"
				case "drive":
					data["CapacityBytes"], data["MediaType"], data["Protocol"] = int64(1920383410176), "SSD", "NVMe"
				case "volume":
					data["CapacityBytes"], data["RAIDType"] = int64(3840766820352), "RAID1"
				case "firmware", "software":
					data["Version"], data["ReleaseDate"] = "1.2.3", "2025-03-04T10:30:00Z"
				case "assembly":
					data["Version"], data["EngineeringChangeLevel"] = "3", "4"
				}
				resources[i] = &Resource{
					Kind: kind,
					Key:  name,
					URI:  "/redfish/v1/" + name,
					Doc: Document{
						Name: name,
					},
					AcquisitionState: "readable",
					Data:             data,
				}
			}
			p := New("https://bmc.example.test", nil)
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
