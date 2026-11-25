// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strconv"
	"strings"
)

type observations struct {
	cpu             sample
	memoryUsed      sample
	memoryAvailable sample
	interfaces      []networkTraffic
	drives          []diskSpace
}

type sample struct {
	value uint64
	valid bool
}

type networkTraffic struct {
	name     string
	received sample
	sent     sample
}

type diskSpace struct {
	drive string
	used  sample
	free  sample
}

func (o *observations) readCPU(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	s := readUnsigned(rows[0]["percentprocessortime"])
	if !s.valid || s.value > 100 {
		return false
	}
	o.cpu = s
	return true
}

func (o *observations) readMemory(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	total := readUnsigned(rows[0]["totalvisiblememorysize"])
	free := readUnsigned(rows[0]["freephysicalmemory"])
	o.memoryAvailable = free
	o.memoryUsed = usedSpace(total, free)
	return free.valid
}

func (o *observations) readNetwork(rows []map[string]any) bool {
	for _, row := range rows {
		name, _ := row["name"].(string)
		if strings.TrimSpace(name) == "" || name == "_Total" {
			continue
		}
		received := readUnsigned(row["bytesreceivedpersec"])
		sent := readUnsigned(row["bytessentpersec"])
		if received.valid || sent.valid {
			o.interfaces = append(o.interfaces, networkTraffic{
				name:     name,
				received: received,
				sent:     sent,
			})
		}
	}
	return len(rows) == 0 || len(o.interfaces) > 0
}

func (o *observations) readDisks(rows []map[string]any) bool {
	for _, row := range rows {
		drive, _ := row["deviceid"].(string)
		if strings.TrimSpace(drive) == "" {
			continue
		}
		size := readUnsigned(row["size"])
		free := readUnsigned(row["freespace"])
		if free.valid {
			o.drives = append(o.drives, diskSpace{
				drive: drive,
				used:  usedSpace(size, free),
				free:  free,
			})
		}
	}
	return len(rows) == 0 || len(o.drives) > 0
}

func usedSpace(total, free sample) sample {
	if total.valid && free.valid && total.value >= free.value {
		return sample{
			value: total.value - free.value,
			valid: true,
		}
	}
	return sample{}
}

func readUnsigned(value any) sample {
	switch v := value.(type) {
	case uint32:
		return sample{
			value: uint64(v),
			valid: true,
		}
	case uint64:
		return sample{
			value: v,
			valid: true,
		}
	case int32:
		if v >= 0 {
			return sample{
				value: uint64(v),
				valid: true,
			}
		}
	case int64:
		if v >= 0 {
			return sample{
				value: uint64(v),
				valid: true,
			}
		}
	case string:
		n, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return sample{
				value: n,
				valid: true,
			}
		}
	}
	return sample{}
}
