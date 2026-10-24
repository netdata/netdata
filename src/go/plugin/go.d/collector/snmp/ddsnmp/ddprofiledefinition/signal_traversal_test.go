// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import "testing"

// Signal traversal has fixed schema-size work and must not retain row state.
// ns/op is a local trend indicator; allocation counts expose traversal regressions.
func BenchmarkSignalTraversal(b *testing.B) {
	b.Run("BGP", func(b *testing.B) {
		row := BGPConfig{
			State: BGPStateConfig{
				BGPValueConfig: BGPValueConfig{
					Value: "established",
				},
			},
			Traffic: BGPTrafficConfig{
				Messages: BGPDirectionalConfig{
					Received: BGPValueConfig{
						From: "1.2.3",
					},
					Sent: BGPValueConfig{
						From: "1.2.4",
					},
				},
			},
		}
		b.ReportAllocs()
		for b.Loop() {
			ForEachBGPSignalValue(row, func(_ string, _ BGPValueConfig) {})
		}
	})
	b.Run("Licensing", func(b *testing.B) {
		row := LicensingConfig{
			State: LicenseStateConfig{
				LicenseValueConfig: LicenseValueConfig{
					Value: "ok",
				},
			},
		}
		b.ReportAllocs()
		for b.Loop() {
			ForEachLicenseSignalValue(row, func(LicenseValueConfig) {})
		}
	})
}

func BenchmarkLicenseMergeIdentity(b *testing.B) {
	row := LicensingConfig{State: LicenseStateConfig{LicenseValueConfig: LicenseValueConfig{From: "1.2.3.0"}}}
	b.ReportAllocs()
	for b.Loop() {
		_ = LicenseMergeIdentity(row)
	}
}
