// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/oschwald/maxminddb-golang"
	"go4.org/netipx"
)

type dbipAsnMMDBRecord struct {
	AutonomousSystemNumber       *uint32 `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string  `maxminddb:"autonomous_system_organization"`
}

type dbipCountryMMDBValue struct {
	ISOCode string `maxminddb:"iso_code"`
}

type dbipCityMMDBValue struct {
	Names map[string]string `maxminddb:"names"`
}

type dbipSubdivisionMMDBValue struct {
	ISOCode string            `maxminddb:"iso_code"`
	Names   map[string]string `maxminddb:"names"`
}

type dbipLocationMMDBValue struct {
	Latitude  float64 `maxminddb:"latitude"`
	Longitude float64 `maxminddb:"longitude"`
}

type dbipGeoMMDBRecord struct {
	Country      *dbipCountryMMDBValue      `maxminddb:"country"`
	City         *dbipCityMMDBValue         `maxminddb:"city"`
	Subdivisions []dbipSubdivisionMMDBValue `maxminddb:"subdivisions"`
	Region       string                     `maxminddb:"region"`
	Location     *dbipLocationMMDBValue     `maxminddb:"location"`
}

func loadRanges(
	cfg config,
	dl *downloader,
) ([]asnRange, []geoRange, []generationDatasetRef, error) {
	asnSources := make([][]asnRange, 0)
	geoSources := make([][]geoRange, 0)
	sourceRefs := make([]generationDatasetRef, 0, len(cfg.sources))

	for i, source := range cfg.sources {
		ref, content, err := dl.readDataset(source)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("source %d (%s): %w", i, sourceLabel(source, i), err)
		}
		sourceRefs = append(sourceRefs, ref)

		switch source.family {
		case sourceFamilyASN:
			asnRanges, err := parseASNSource(source, content)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("source %d (%s): %w", i, sourceLabel(source, i), err)
			}
			asnSources = append(asnSources, asnRanges)
		case sourceFamilyGeo:
			geoRanges, err := parseGeoSource(source, content)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("source %d (%s): %w", i, sourceLabel(source, i), err)
			}
			geoSources = append(geoSources, geoRanges)
		default:
			return nil, nil, nil, fmt.Errorf(
				"source %d (%s): unsupported family %q",
				i,
				sourceLabel(source, i),
				source.family,
			)
		}
	}

	mergedASN, err := mergeAsnSources(asnSources)
	if err != nil {
		return nil, nil, nil, err
	}
	mergedGeo, err := mergeGeoSources(geoSources)
	if err != nil {
		return nil, nil, nil, err
	}

	return mergedASN, mergedGeo, sourceRefs, nil
}

func sourceLabel(source sourceEntry, index int) string {
	if source.name != "" {
		return source.name
	}
	return fmt.Sprintf("%s-%d", source.family, index)
}

func parseASNSource(source sourceEntry, payload []byte) ([]asnRange, error) {
	switch {
	case source.provider == providerIPToASN && source.artifact == artifactIPToASNCombined:
		if source.format != formatTSV {
			return nil, fmt.Errorf("iptoasn combined requires tsv format, got %q", source.format)
		}
		return parseIPToASNCombinedTSVAsn(payload)
	case source.provider == providerDBIP && source.artifact == artifactDBIPASNLite:
		switch source.format {
		case formatCSV:
			return parseDBIPAsnCSV(payload)
		case formatMMDB:
			return parseDBIPAsnMMDB(payload)
		default:
			return nil, fmt.Errorf("unsupported dbip ASN format %q", source.format)
		}
	default:
		return nil, fmt.Errorf(
			"unsupported ASN source %q/%q",
			source.provider,
			source.artifact,
		)
	}
}

func parseGeoSource(source sourceEntry, payload []byte) ([]geoRange, error) {
	switch {
	case source.provider == providerIPToASN && source.artifact == artifactIPToASNCombined:
		if source.format != formatTSV {
			return nil, fmt.Errorf("iptoasn combined requires tsv format, got %q", source.format)
		}
		return parseIPToASNCombinedTSVGeo(payload)
	case source.provider == providerDBIP && source.artifact == artifactDBIPCountryLite:
		switch source.format {
		case formatCSV:
			return parseDBIPCountryCSV(payload)
		case formatMMDB:
			return parseDBIPGeoMMDB(payload)
		default:
			return nil, fmt.Errorf("unsupported dbip GEO format %q", source.format)
		}
	case source.provider == providerDBIP && source.artifact == artifactDBIPCityLite:
		switch source.format {
		case formatCSV:
			return parseDBIPCityCSV(payload)
		case formatMMDB:
			return parseDBIPGeoMMDB(payload)
		default:
			return nil, fmt.Errorf("unsupported dbip GEO format %q", source.format)
		}
	default:
		return nil, fmt.Errorf(
			"unsupported GEO source %q/%q",
			source.provider,
			source.artifact,
		)
	}
}

func parseIPToASNCombinedTSVAsn(payload []byte) ([]asnRange, error) {
	asnRanges := make([]asnRange, 0, 1<<20)

	scanner := bufio.NewScanner(bytes.NewReader(payload))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			return nil, fmt.Errorf("iptoasn line %d: expected at least 5 columns", lineNo)
		}
		start, end, err := parseRangeEndpoints(parts[0], parts[1])
		if err != nil {
			return nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		asn, err := parseASN(parts[2])
		if err != nil {
			return nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		org := strings.TrimSpace(parts[4])

		asnRange := asnRange{start: start, end: end, asn: asn, org: org}
		if err := asnRange.validate(); err != nil {
			return nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		asnRanges = append(asnRanges, asnRange)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan iptoasn payload: %w", err)
	}
	return asnRanges, nil
}

func parseIPToASNCombinedTSVGeo(payload []byte) ([]geoRange, error) {
	geoRanges := make([]geoRange, 0, 1<<20)

	scanner := bufio.NewScanner(bytes.NewReader(payload))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			return nil, fmt.Errorf("iptoasn line %d: expected at least 5 columns", lineNo)
		}
		start, end, err := parseRangeEndpoints(parts[0], parts[1])
		if err != nil {
			return nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		country := normalizeCountry(parts[3])
		if country == "" {
			continue
		}
		rec := geoRange{start: start, end: end, country: country}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		geoRanges = append(geoRanges, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan iptoasn payload: %w", err)
	}
	return geoRanges, nil
}

func parseDBIPAsnCSV(payload []byte) ([]asnRange, error) {
	reader := csv.NewReader(strings.NewReader(string(payload)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	out := make([]asnRange, 0, 1<<18)
	lineNo := 0
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("dbip asn line %d: %w", lineNo+1, err)
		}
		lineNo++
		if len(row) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		if len(row) < 3 {
			return nil, fmt.Errorf("dbip asn line %d: expected >= 3 columns", lineNo)
		}
		start, end, err := parseRangeEndpoints(row[0], row[1])
		if err != nil {
			return nil, fmt.Errorf("dbip asn line %d: %w", lineNo, err)
		}
		asn, err := parseASN(row[2])
		if err != nil {
			return nil, fmt.Errorf("dbip asn line %d: %w", lineNo, err)
		}
		org := ""
		if len(row) > 3 {
			org = strings.TrimSpace(row[3])
		}
		rec := asnRange{start: start, end: end, asn: asn, org: org}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("dbip asn line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseDBIPCountryCSV(payload []byte) ([]geoRange, error) {
	reader := csv.NewReader(strings.NewReader(string(payload)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	out := make([]geoRange, 0, 1<<18)
	lineNo := 0
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("dbip country line %d: %w", lineNo+1, err)
		}
		lineNo++
		if len(row) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		if len(row) < 3 {
			return nil, fmt.Errorf("dbip country line %d: expected >= 3 columns", lineNo)
		}
		start, end, err := parseRangeEndpoints(row[0], row[1])
		if err != nil {
			return nil, fmt.Errorf("dbip country line %d: %w", lineNo, err)
		}
		country := normalizeCountry(row[2])
		if country == "" {
			continue
		}
		rec := geoRange{start: start, end: end, country: country}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("dbip country line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseDBIPCityCSV(payload []byte) ([]geoRange, error) {
	reader := csv.NewReader(strings.NewReader(string(payload)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	out := make([]geoRange, 0, 1<<18)
	lineNo := 0
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("dbip city line %d: %w", lineNo+1, err)
		}
		lineNo++
		if len(row) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(row[0]), "#") {
			continue
		}
		if len(row) < 8 {
			return nil, fmt.Errorf("dbip city line %d: expected >= 8 columns", lineNo)
		}

		start, end, err := parseRangeEndpoints(row[0], row[1])
		if err != nil {
			return nil, fmt.Errorf("dbip city line %d: %w", lineNo, err)
		}

		country := normalizeCountry(row[3])
		state := strings.TrimSpace(row[4])
		city := strings.TrimSpace(row[5])

		rec := geoRange{
			start:   start,
			end:     end,
			country: country,
			state:   state,
			city:    city,
		}

		if latRaw := strings.TrimSpace(row[6]); latRaw != "" {
			lat, err := strconv.ParseFloat(latRaw, 64)
			if err != nil {
				return nil, fmt.Errorf("dbip city line %d: invalid latitude %q: %w", lineNo, latRaw, err)
			}
			rec.latitude = lat
			rec.hasLocation = true
		}
		if lonRaw := strings.TrimSpace(row[7]); lonRaw != "" {
			lon, err := strconv.ParseFloat(lonRaw, 64)
			if err != nil {
				return nil, fmt.Errorf("dbip city line %d: invalid longitude %q: %w", lineNo, lonRaw, err)
			}
			rec.longitude = lon
			rec.hasLocation = true
		}

		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("dbip city line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseDBIPAsnMMDB(payload []byte) ([]asnRange, error) {
	reader, err := maxminddb.FromBytes(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to open dbip ASN mmdb: %w", err)
	}

	out := make([]asnRange, 0, 1<<18)
	networks := reader.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		var record dbipAsnMMDBRecord
		network, err := networks.Network(&record)
		if err != nil {
			return nil, fmt.Errorf("failed to decode dbip ASN network: %w", err)
		}
		start, end, err := rangeFromIPNet(network)
		if err != nil {
			return nil, err
		}
		asn := uint32(0)
		if record.AutonomousSystemNumber != nil {
			asn = *record.AutonomousSystemNumber
		}
		rec := asnRange{
			start: start,
			end:   end,
			asn:   asn,
			org:   strings.TrimSpace(record.AutonomousSystemOrganization),
		}
		if rec.asn == 0 && rec.org == "" {
			continue
		}
		if err := rec.validate(); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := networks.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate dbip ASN mmdb: %w", err)
	}
	return out, nil
}

func parseDBIPGeoMMDB(payload []byte) ([]geoRange, error) {
	reader, err := maxminddb.FromBytes(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to open dbip GEO mmdb: %w", err)
	}

	out := make([]geoRange, 0, 1<<18)
	networks := reader.Networks(maxminddb.SkipAliasedNetworks)
	for networks.Next() {
		var record dbipGeoMMDBRecord
		network, err := networks.Network(&record)
		if err != nil {
			return nil, fmt.Errorf("failed to decode dbip GEO network: %w", err)
		}
		start, end, err := rangeFromIPNet(network)
		if err != nil {
			return nil, err
		}

		rec := geoRange{
			start:   start,
			end:     end,
			country: dbipCountryCode(record.Country),
			state:   dbipStateName(record),
			city:    dbipCityName(record.City),
		}
		if record.Location != nil {
			rec.latitude = record.Location.Latitude
			rec.longitude = record.Location.Longitude
			rec.hasLocation = true
		}
		if rec.country == "" && rec.state == "" && rec.city == "" && !rec.hasLocation {
			continue
		}
		if err := rec.validate(); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := networks.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate dbip GEO mmdb: %w", err)
	}
	return out, nil
}

func dbipCountryCode(value *dbipCountryMMDBValue) string {
	if value == nil {
		return ""
	}
	return normalizeCountry(value.ISOCode)
}

func dbipCityName(value *dbipCityMMDBValue) string {
	if value == nil {
		return ""
	}
	if name := strings.TrimSpace(value.Names["en"]); name != "" {
		return name
	}
	for _, name := range value.Names {
		name = strings.TrimSpace(name)
		if name != "" {
			return name
		}
	}
	return ""
}

func dbipStateName(record dbipGeoMMDBRecord) string {
	if len(record.Subdivisions) > 0 {
		if name := strings.TrimSpace(record.Subdivisions[0].Names["en"]); name != "" {
			return name
		}
		for _, name := range record.Subdivisions[0].Names {
			name = strings.TrimSpace(name)
			if name != "" {
				return name
			}
		}
		if code := strings.TrimSpace(record.Subdivisions[0].ISOCode); code != "" {
			return code
		}
	}
	return strings.TrimSpace(record.Region)
}

func rangeFromIPNet(network *net.IPNet) (netip.Addr, netip.Addr, error) {
	prefix, err := netip.ParsePrefix(network.String())
	if err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf(
			"failed to parse mmdb network %s: %w",
			network.String(),
			err,
		)
	}
	rng := netipx.RangeOfPrefix(prefix.Masked())
	return rng.From(), rng.To(), nil
}

func parseRangeEndpoints(startRaw, endRaw string) (netip.Addr, netip.Addr, error) {
	start, err := parseIP(startRaw)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("invalid start address %q: %w", startRaw, err)
	}
	end, err := parseIP(endRaw)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("invalid end address %q: %w", endRaw, err)
	}
	if start.BitLen() != end.BitLen() {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("mixed address families %q and %q", startRaw, endRaw)
	}
	if compareAddrs(start, end) > 0 {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("start %s after end %s", start, end)
	}
	return start, end, nil
}

func parseIP(raw string) (netip.Addr, error) {
	value := strings.TrimSpace(strings.Trim(raw, "\""))
	if value == "" {
		return netip.Addr{}, fmt.Errorf("empty ip")
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.Unmap(), nil
	}

	if num, ok := new(big.Int).SetString(value, 10); ok {
		if num.Sign() < 0 {
			return netip.Addr{}, fmt.Errorf("negative integer address")
		}
		if num.BitLen() <= 32 {
			v := uint32(num.Uint64())
			return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), nil
		}
		if num.BitLen() <= 128 {
			b := num.FillBytes(make([]byte, 16))
			var arr [16]byte
			copy(arr[:], b)
			return netip.AddrFrom16(arr), nil
		}
	}

	return netip.Addr{}, fmt.Errorf("unsupported ip value")
}

func parseASN(raw string) (uint32, error) {
	value := strings.TrimSpace(strings.Trim(raw, "\""))
	if value == "" {
		return 0, nil
	}
	if strings.HasPrefix(value, "AS") || strings.HasPrefix(value, "as") {
		value = value[2:]
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid ASN %q: %w", raw, err)
	}
	return uint32(n), nil
}
