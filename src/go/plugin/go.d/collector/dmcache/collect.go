// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type dmCacheDevice struct {
	name                  string
	metaBlockSizeSectors  int64
	metaUsedBlocks        int64
	metaTotalBlocks       int64
	cacheBlockSizeSectors int64
	cacheUsedBlocks       int64
	cacheTotalBlocks      int64
	readHits              int64
	readMisses            int64
	writeHits             int64
	writeMisses           int64
	demotionsBlocks       int64
	promotionsBlocks      int64
	dirtyBlocks           int64
}

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.cacheStatus()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	if err := c.collectCacheStatus(mx, bs); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectCacheStatus(mx map[string]int64, data []byte) error {
	var devices []*dmCacheDevice

	sc := bufio.NewScanner(bytes.NewReader(data))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		dev, err := parseDmsetupStatusLine(line)
		if err != nil {
			return fmt.Errorf("malformed dmsetup status line: %v ('%s')", err, line)
		}

		devices = append(devices, dev)
	}

	seen := make(map[string]bool)

	for _, dev := range devices {
		seen[dev.name] = true

		if !c.devices[dev.name] {
			c.devices[dev.name] = true
			c.addDeviceCharts(dev.name)
		}

		px := fmt.Sprintf("dmcache_device_%s_", dev.name)

		const sectorSize = 512
		metaMul := dev.metaBlockSizeSectors * sectorSize
		cacheMul := dev.cacheBlockSizeSectors * sectorSize

		mx[px+"metadata_free_bytes"] = (dev.metaTotalBlocks - dev.metaUsedBlocks) * metaMul
		mx[px+"metadata_used_bytes"] = dev.metaUsedBlocks * metaMul
		mx[px+"cache_free_bytes"] = (dev.cacheTotalBlocks - dev.cacheUsedBlocks) * cacheMul
		mx[px+"cache_used_bytes"] = dev.cacheUsedBlocks * cacheMul
		mx[px+"read_hits"] = dev.readHits
		mx[px+"read_misses"] = dev.readMisses
		mx[px+"write_hits"] = dev.writeHits
		mx[px+"write_misses"] = dev.writeMisses
		mx[px+"demotions_bytes"] = dev.demotionsBlocks * cacheMul
		mx[px+"promotions_bytes"] = dev.promotionsBlocks * cacheMul
		mx[px+"dirty_bytes"] = dev.dirtyBlocks * cacheMul
	}

	for dev := range c.devices {
		if !seen[dev] {
			delete(c.devices, dev)
			c.removeDeviceCharts(dev)
		}
	}

	if len(devices) == 0 {
		return errors.New("no dm-cache devices found")
	}

	return nil
}

func parseDmsetupStatusLine(line string) (*dmCacheDevice, error) {
	// https://www.kernel.org/doc/html/next/admin-guide/device-mapper/cache.html#status

	parts := strings.Fields(line)
	if len(parts) < 15 {
		return nil, fmt.Errorf("want at least 15 fields, got %d", len(parts))
	}

	var dev dmCacheDevice
	var err error

	for i, s := range parts {
		switch i {
		case 0:
			dev.name = strings.TrimSuffix(parts[0], ":")
		case 4:
			dev.metaBlockSizeSectors, err = parseInt(s)
		case 5:
			dev.metaUsedBlocks, dev.metaTotalBlocks, err = parseUsedTotalBlocks(s)
		case 6:
			dev.cacheBlockSizeSectors, err = parseInt(s)
		case 7:
			dev.cacheUsedBlocks, dev.cacheTotalBlocks, err = parseUsedTotalBlocks(s)
		case 8:
			dev.readHits, err = parseInt(s)
		case 9:
			dev.readMisses, err = parseInt(s)
		case 10:
			dev.writeHits, err = parseInt(s)
		case 11:
			dev.writeMisses, err = parseInt(s)
		case 12:
			dev.demotionsBlocks, err = parseInt(s)
		case 13:
			dev.promotionsBlocks, err = parseInt(s)
		case 14:
			dev.dirtyBlocks, err = parseInt(s)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse %d field '%s': %v", i, s, err)
		}
	}

	return &dev, nil
}

func parseUsedTotalBlocks(info string) (int64, int64, error) {
	parts := strings.Split(info, "/")
	if len(parts) != 2 {
		return 0, 0, errors.New("expected used/total")
	}
	used, err := parseInt(parts[0])
	if err != nil {
		return 0, 0, err
	}
	total, err := parseInt(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return used, total, nil
}

func parseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
