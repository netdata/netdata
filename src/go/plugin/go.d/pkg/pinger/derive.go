// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

func deriveSample(host string, stats *probing.Statistics, track bool, states *stateStore, cfg AnalysisConfig) Sample {
	sample := Sample{
		Host:          host,
		PacketsSent:   int64(stats.PacketsSent),
		PacketsRecv:   int64(stats.PacketsRecv),
		PacketLossPct: stats.PacketLoss,
		RTT:           deriveRTT(stats),
	}

	sample.Jitter = deriveJitter(host, stats, track, states, cfg)

	return sample
}

func deriveRTT(stats *probing.Statistics) RTTSummary {
	if stats.PacketsRecv == 0 {
		return RTTSummary{}
	}

	return RTTSummary{
		Valid:  true,
		Min:    stats.MinRtt,
		Max:    stats.MaxRtt,
		Avg:    stats.AvgRtt,
		StdDev: stats.StdDevRtt,
	}
}

func deriveJitter(host string, stats *probing.Statistics, track bool, states *stateStore, cfg AnalysisConfig) JitterSummary {
	if len(stats.Rtts) < 2 {
		return JitterSummary{}
	}

	mean := calcMeanJitter(stats.Rtts)
	js := JitterSummary{
		InstantValid: true,
		Mean:         mean,
	}

	if !track {
		return js
	}

	ewma, sma := states.update(host, mean, cfg)
	js.SmoothedValid = true
	js.EWMA = ewma
	js.SMA = sma

	return js
}

func calcMeanJitter(rtts []time.Duration) time.Duration {
	if len(rtts) < 2 {
		return 0
	}

	var sum int64
	for i := 1; i < len(rtts); i++ {
		diff := rtts[i] - rtts[i-1]
		if diff < 0 {
			diff = -diff
		}
		sum += int64(diff)
	}

	return time.Duration(sum / int64(len(rtts)-1))
}
