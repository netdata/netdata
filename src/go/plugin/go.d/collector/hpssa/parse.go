// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

type hpssaController struct {
	model                   string
	slot                    string
	serialNumber            string
	controllerStatus        string
	cacheBoardPresent       string
	cacheStatus             string
	cacheRatio              string
	batteryCapacitorCount   string
	batteryCapacitorStatus  string
	controllerTemperatureC  string
	cacheModuleTemperatureC string
	numberOfPorts           string
	driverName              string
	arrays                  map[string]*hpssaArray
	unassignedDrives        map[string]*hpssaPhysicalDrive
}

func (c *hpssaController) uniqueKey() string {
	return fmt.Sprintf("%s/%s", c.model, c.slot)
}

type hpssaArray struct {
	cntrl *hpssaController

	id            string
	interfaceType string
	unusedSpace   string
	usedSpace     string
	status        string
	arrayType     string
	logicalDrives map[string]*hpssaLogicalDrive
}

func (a *hpssaArray) uniqueKey() string {
	return fmt.Sprintf("%s/%s/%s", a.cntrl.model, a.cntrl.slot, a.id)
}

type hpssaLogicalDrive struct {
	cntrl *hpssaController
	arr   *hpssaArray

	id                string
	size              string
	status            string
	diskName          string
	uniqueIdentifier  string
	logicalDriveLabel string
	driveType         string
	physicalDrives    map[string]*hpssaPhysicalDrive
}

func (ld *hpssaLogicalDrive) uniqueKey() string {
	return fmt.Sprintf("%s/%s/%s/%s", ld.cntrl.model, ld.cntrl.slot, ld.arr.id, ld.id)
}

type hpssaPhysicalDrive struct {
	cntrl *hpssaController
	arr   *hpssaArray
	ld    *hpssaLogicalDrive

	location            string // port:box:bay
	status              string
	driveType           string
	interfaceType       string
	size                string
	serialNumber        string
	wwid                string
	model               string
	currentTemperatureC string
}

func (pd *hpssaPhysicalDrive) uniqueKey() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", pd.cntrl.model, pd.cntrl.slot, pd.arrId(), pd.ldId(), pd.location)
}

func (pd *hpssaPhysicalDrive) arrId() string {
	if pd.arr == nil {
		return "na"
	}
	return pd.arr.id
}

func (pd *hpssaPhysicalDrive) ldId() string {
	if pd.ld == nil {
		return "na"
	}
	return pd.ld.id
}

func parseSsacliControllersInfo(data []byte) (map[string]*hpssaController, error) {
	var (
		cntrl *hpssaController
		arr   *hpssaArray
		ld    *hpssaLogicalDrive
		pd    *hpssaPhysicalDrive

		line       string
		prevLine   string
		section    string
		unassigned bool
	)

	controllers := make(map[string]*hpssaController)

	sc := bufio.NewScanner(bytes.NewReader(data))

	for sc.Scan() {
		prevLine = line
		line = sc.Text()

		switch {
		case line == "":
			section = ""
			continue
		case strings.HasPrefix(line, "HPE Smart Array"), strings.HasPrefix(line, "Smart Array"):
			section = "controller"

			v, err := parseControllerLine(line)
			if err != nil {
				return nil, err
			}

			cntrl = v
			controllers[cntrl.slot] = cntrl

			continue
		case strings.HasPrefix(line, "   Array:") && cntrl != nil:
			section = "array"
			unassigned = false

			arr = parseArrayLine(line)
			cntrl.arrays[arr.id] = arr

			continue
		case strings.HasPrefix(line, "      Logical Drive:") && cntrl != nil && arr != nil:
			section = "logical drive"

			ld = parseLogicalDriveLine(line)
			arr.logicalDrives[arr.id] = ld

			continue
		case strings.HasPrefix(line, "      physicaldrive") && prevLine == "":
			section = "physical drive"

			if unassigned && cntrl == nil {
				return nil, fmt.Errorf("unassigned drive but controller is nil (line '%s')", line)
			}
			if !unassigned && ld == nil {
				return nil, fmt.Errorf("assigned drive but logical device is nil (line '%s')", line)
			}

			v, err := parsePhysicalDriveLine(line)
			if err != nil {
				return nil, err
			}

			pd = v
			if unassigned {
				cntrl.unassignedDrives[pd.location] = pd
			} else {
				ld.physicalDrives[pd.location] = pd
			}

			continue
		case strings.HasPrefix(line, "   Unassigned"):
			unassigned = true
			continue
		}

		switch section {
		case "controller":
			parseControllerSectionLine(line, cntrl)
		case "array":
			parseArraySectionLine(line, arr)
		case "logical drive":
			parseLogicalDriveSectionLine(line, ld)
		case "physical drive":
			parsePhysicalDriveSectionLine(line, pd)
		}
	}

	if len(controllers) == 0 {
		return nil, fmt.Errorf("no controllers found")
	}

	updateHpssaHierarchy(controllers)

	return controllers, nil
}

func updateHpssaHierarchy(controllers map[string]*hpssaController) {
	for _, cntrl := range controllers {
		for _, pd := range cntrl.unassignedDrives {
			pd.cntrl = cntrl
		}
		for _, arr := range cntrl.arrays {
			arr.cntrl = cntrl
			for _, ld := range arr.logicalDrives {
				ld.cntrl = cntrl
				ld.arr = arr
				for _, pd := range ld.physicalDrives {
					pd.cntrl = cntrl
					pd.arr = arr
					pd.ld = ld
				}
			}
		}
	}
}

func parseControllerLine(line string) (*hpssaController, error) {
	parts := strings.Fields(strings.TrimPrefix(line, "Smart Array "))
	if len(parts) < 4 {
		return nil, fmt.Errorf("malformed Smart Array line: '%s'", line)
	}

	cntrl := &hpssaController{
		model:            parts[0],
		slot:             parts[3],
		arrays:           make(map[string]*hpssaArray),
		unassignedDrives: make(map[string]*hpssaPhysicalDrive),
	}

	return cntrl, nil
}

func parseArrayLine(line string) *hpssaArray {
	arr := &hpssaArray{
		id:            getColonSepValue(line),
		logicalDrives: make(map[string]*hpssaLogicalDrive),
	}

	return arr
}

func parseLogicalDriveLine(line string) *hpssaLogicalDrive {
	ld := &hpssaLogicalDrive{
		id:             getColonSepValue(line),
		physicalDrives: make(map[string]*hpssaPhysicalDrive),
	}

	return ld
}

func parsePhysicalDriveLine(line string) (*hpssaPhysicalDrive, error) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed physicaldrive line: '%s'", line)
	}

	pd := &hpssaPhysicalDrive{
		location: parts[1],
	}

	return pd, nil
}

func parseControllerSectionLine(line string, cntrl *hpssaController) {
	indent := strings.Repeat(" ", 3)

	switch {
	case strings.HasPrefix(line, indent+"Serial Number:"):
		cntrl.serialNumber = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Controller Status:"):
		cntrl.controllerStatus = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Cache Board Present:"):
		cntrl.cacheBoardPresent = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Cache Status:"):
		cntrl.cacheStatus = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Cache Ratio:"):
		cntrl.cacheRatio = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Controller Temperature (C):"):
		cntrl.controllerTemperatureC = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Cache Module Temperature (C):"):
		cntrl.cacheModuleTemperatureC = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Number of Ports:"):
		cntrl.numberOfPorts = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Driver Name:"):
		cntrl.driverName = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Battery/Capacitor Count:"):
		cntrl.batteryCapacitorCount = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Battery/Capacitor Status:"):
		cntrl.batteryCapacitorStatus = getColonSepValue(line)
	}
}

func parseArraySectionLine(line string, arr *hpssaArray) {
	indent := strings.Repeat(" ", 6)

	switch {
	case strings.HasPrefix(line, indent+"Interface Type:"):
		arr.interfaceType = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Unused Space:"):
		arr.unusedSpace = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Used Space:"):
		arr.usedSpace = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Status:"):
		arr.status = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Array Type:"):
		arr.arrayType = getColonSepValue(line)
	}
}

func parseLogicalDriveSectionLine(line string, ld *hpssaLogicalDrive) {
	indent := strings.Repeat(" ", 9)

	switch {
	case strings.HasPrefix(line, indent+"Size:"):
		ld.size = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Status:"):
		ld.status = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Disk Name:"):
		ld.diskName = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Unique Identifier:"):
		ld.uniqueIdentifier = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Logical Drive Label:"):
		ld.logicalDriveLabel = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Drive Type:"):
		ld.driveType = getColonSepValue(line)
	}
}

func parsePhysicalDriveSectionLine(line string, pd *hpssaPhysicalDrive) {
	indent := strings.Repeat(" ", 9)

	switch {
	case strings.HasPrefix(line, indent+"Status:"):
		pd.status = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Drive Type:"):
		pd.driveType = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Interface Type:"):
		pd.interfaceType = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Size:"):
		pd.size = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Serial Number:"):
		pd.serialNumber = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"WWID:"):
		pd.wwid = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Model:"):
		pd.model = getColonSepValue(line)
	case strings.HasPrefix(line, indent+"Current Temperature (C):"):
		pd.currentTemperatureC = getColonSepValue(line)
	}
}

func getColonSepValue(line string) string {
	i := strings.IndexByte(line, ':')
	if i == -1 {
		return ""
	}
	return strings.TrimSpace(line[i+1:])
}
