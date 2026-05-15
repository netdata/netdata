// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

// collectHardwareHealth classifies hardware components by lifecycle state.
// Uses cached hardware data from discovery to avoid redundant API calls.
func (c *Collector) collectHardwareHealth() {
	var fanOK, fanDeg, fanFail, fanUnk float64
	var psuOK, psuDeg, psuFail, psuUnk float64
	var drvOK, drvDeg, drvFail, drvUnk float64
	var batOK, batDeg, batFail, batUnk float64
	var nodOK, nodDeg, nodFail, nodUnk float64

	for _, h := range c.discovered.hardware {
		ok, deg, fail, unk := classifyState(h.LifecycleState)
		switch h.Type {
		case "Fan":
			fanOK += ok
			fanDeg += deg
			fanFail += fail
			fanUnk += unk
		case "Power_Supply":
			psuOK += ok
			psuDeg += deg
			psuFail += fail
			psuUnk += unk
		case "Drive":
			drvOK += ok
			drvDeg += deg
			drvFail += fail
			drvUnk += unk
		case "Battery":
			batOK += ok
			batDeg += deg
			batFail += fail
			batUnk += unk
		case "Node":
			nodOK += ok
			nodDeg += deg
			nodFail += fail
			nodUnk += unk
		}
	}

	hw := c.mx.hardware
	hw.fanOK.Observe(fanOK)
	hw.fanDegraded.Observe(fanDeg)
	hw.fanFailed.Observe(fanFail)
	hw.fanUnknown.Observe(fanUnk)
	hw.psuOK.Observe(psuOK)
	hw.psuDegraded.Observe(psuDeg)
	hw.psuFailed.Observe(psuFail)
	hw.psuUnknown.Observe(psuUnk)
	hw.driveOK.Observe(drvOK)
	hw.driveDegraded.Observe(drvDeg)
	hw.driveFailed.Observe(drvFail)
	hw.driveUnknown.Observe(drvUnk)
	hw.batteryOK.Observe(batOK)
	hw.batteryDegraded.Observe(batDeg)
	hw.batteryFailed.Observe(batFail)
	hw.batteryUnknown.Observe(batUnk)
	hw.nodeOK.Observe(nodOK)
	hw.nodeDegraded.Observe(nodDeg)
	hw.nodeFailed.Observe(nodFail)
	hw.nodeUnknown.Observe(nodUnk)
}

func classifyState(state string) (ok, degraded, failed, unknown float64) {
	switch state {
	case "Healthy", "healthy":
		return 1, 0, 0, 0
	case "Degraded", "degraded":
		return 0, 1, 0, 0
	case "Failed", "failed", "Error", "error":
		return 0, 0, 1, 0
	default:
		return 0, 0, 0, 1
	}
}
