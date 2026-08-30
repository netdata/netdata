// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

var verificationStates = []string{"verified", "unverified"}

type profileMetrics struct {
	followers    metrix.SnapshotGaugeVec
	following    metrix.SnapshotGaugeVec
	statuses     metrix.SnapshotCounterVec
	verification metrix.SnapshotStateSetVec
}

func newProfileMetrics(store metrix.CollectorStore) profileMetrics {
	vec := store.Write().SnapshotMeter("").Vec("user_id", "username", "name")
	return profileMetrics{
		followers: vec.Gauge("followers"),
		following: vec.Gauge("following"),
		statuses:  vec.Counter("statuses_total"),
		verification: vec.StateSet(
			"verification_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(verificationStates...),
		),
	}
}

func (m profileMetrics) write(p profile) {
	labels := []string{p.ID, p.Username, p.Name}
	if p.Followers != nil {
		m.followers.WithLabelValues(labels...).Observe(float64(*p.Followers))
	}
	if p.Following != nil {
		m.following.WithLabelValues(labels...).Observe(float64(*p.Following))
	}
	if p.StatusesCount != nil {
		m.statuses.WithLabelValues(labels...).ObserveTotal(float64(*p.StatusesCount))
	}
	if p.Verified != nil {
		state := "unverified"
		if *p.Verified {
			state = "verified"
		}
		m.verification.WithLabelValues(labels...).Enable(state)
	}
}
