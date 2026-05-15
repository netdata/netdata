// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

// collectHardwareHealth classifies hardware components by health state.
// Uses cached discovery data — no API calls.
//
// health-numeric mapping: 0=OK, 1=Degraded, 2=Fault, 3=Unknown.
// Special cases:
//   - Drives with usage-numeric=3 (Global Hot Spare) report health=Unknown but are functionally OK.
//   - FRUs use fru-status-numeric: 0=OK, 1=Degraded, 2=Fault, 3=Unknown, 5=N/A (treated as OK).
//   - Ports with health-numeric=4 (Not Available) are excluded from counts.
func (c *Collector) collectHardwareHealth() {
	var ctrlOK, ctrlDeg, ctrlFault, ctrlUnk float64
	var drvOK, drvDeg, drvFault, drvUnk float64
	var fanOK, fanDeg, fanFault, fanUnk float64
	var psuOK, psuDeg, psuFault, psuUnk float64
	var fruOK, fruDeg, fruFault, fruUnk float64
	var portOK, portDeg, portFault, portUnk float64

	for _, ctrl := range c.discovered.controllers {
		ok, deg, fault, unk := classifyHealth(ctrl.HealthNumeric)
		ctrlOK += ok
		ctrlDeg += deg
		ctrlFault += fault
		ctrlUnk += unk
	}

	for _, d := range c.discovered.drives {
		// Global Hot Spare drives report health=Unknown but are functionally OK.
		if d.UsageNumeric == 3 && d.HealthNumeric == 3 {
			drvOK++
			continue
		}
		ok, deg, fault, unk := classifyHealth(d.HealthNumeric)
		drvOK += ok
		drvDeg += deg
		drvFault += fault
		drvUnk += unk
	}

	for _, f := range c.discovered.fans {
		ok, deg, fault, unk := classifyHealth(f.HealthNumeric)
		fanOK += ok
		fanDeg += deg
		fanFault += fault
		fanUnk += unk
	}

	for _, p := range c.discovered.psus {
		ok, deg, fault, unk := classifyHealth(p.HealthNumeric)
		psuOK += ok
		psuDeg += deg
		psuFault += fault
		psuUnk += unk
	}

	for _, f := range c.discovered.frus {
		ok, deg, fault, unk := classifyFRUHealth(f.FRUStatusNum)
		fruOK += ok
		fruDeg += deg
		fruFault += fault
		fruUnk += unk
	}

	for _, p := range c.discovered.ports {
		// 4 = Not Available — exclude from health counts.
		if p.HealthNumeric == 4 {
			continue
		}
		ok, deg, fault, unk := classifyHealth(p.HealthNumeric)
		portOK += ok
		portDeg += deg
		portFault += fault
		portUnk += unk
	}

	hw := c.mx.hardware
	hw.controllerOK.Observe(ctrlOK)
	hw.controllerDegraded.Observe(ctrlDeg)
	hw.controllerFault.Observe(ctrlFault)
	hw.controllerUnknown.Observe(ctrlUnk)
	hw.driveOK.Observe(drvOK)
	hw.driveDegraded.Observe(drvDeg)
	hw.driveFault.Observe(drvFault)
	hw.driveUnknown.Observe(drvUnk)
	hw.fanOK.Observe(fanOK)
	hw.fanDegraded.Observe(fanDeg)
	hw.fanFault.Observe(fanFault)
	hw.fanUnknown.Observe(fanUnk)
	hw.psuOK.Observe(psuOK)
	hw.psuDegraded.Observe(psuDeg)
	hw.psuFault.Observe(psuFault)
	hw.psuUnknown.Observe(psuUnk)
	hw.fruOK.Observe(fruOK)
	hw.fruDegraded.Observe(fruDeg)
	hw.fruFault.Observe(fruFault)
	hw.fruUnknown.Observe(fruUnk)
	hw.portOK.Observe(portOK)
	hw.portDegraded.Observe(portDeg)
	hw.portFault.Observe(portFault)
	hw.portUnknown.Observe(portUnk)
}

// classifyHealth maps health-numeric (0=OK, 1=Degraded, 2=Fault, 3=Unknown).
func classifyHealth(h int) (ok, degraded, fault, unknown float64) {
	switch h {
	case 0:
		return 1, 0, 0, 0
	case 1:
		return 0, 1, 0, 0
	case 2:
		return 0, 0, 1, 0
	default:
		return 0, 0, 0, 1
	}
}

// classifyFRUHealth maps fru-status-numeric (0=OK, 1=Degraded, 2=Fault, 3=Unknown, 5=N/A→OK).
func classifyFRUHealth(h int) (ok, degraded, fault, unknown float64) {
	switch h {
	case 0, 5:
		return 1, 0, 0, 0
	case 1:
		return 0, 1, 0, 0
	case 2:
		return 0, 0, 1, 0
	default:
		return 0, 0, 0, 1
	}
}
