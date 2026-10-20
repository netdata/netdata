// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"path"
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
	case source.provider == providerCAIDA && source.artifact == artifactCAIDAPrefix2AS:
		if source.format != formatTSV {
			return nil, fmt.Errorf("caida prefix2as requires tsv format, got %q", source.format)
		}
		return parseCAIDAPrefix2AS(payload)
	case source.provider == providerMaxMind && source.artifact == artifactMaxMindGeoLite2ASN:
		if source.format != formatMMDB {
			return nil, fmt.Errorf("maxmind geolite2 ASN requires mmdb format, got %q", source.format)
		}
		return parseDBIPAsnMMDB(payload)
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
	case source.provider == providerMaxMind && source.artifact == artifactMaxMindGeoLite2Country:
		if source.format != formatCSV {
			return nil, fmt.Errorf("maxmind geolite2 country requires csv format, got %q", source.format)
		}
		return parseMaxMindCountryCSVZip(payload)
	case source.provider == providerIP2Location && source.artifact == artifactIP2LocationCountryLite:
		if source.format != formatCSV {
			return nil, fmt.Errorf("ip2location country-lite requires csv format, got %q", source.format)
		}
		return parseIP2LocationCountryZip(payload)
	case source.provider == providerIPDeny && source.artifact == artifactIPDenyCountryZones:
		if source.format != formatCIDR {
			return nil, fmt.Errorf("ipdeny country-zones requires cidr format, got %q", source.format)
		}
		return parseIPDenyCountryTarGZ(payload)
	case source.provider == providerIPIP && source.artifact == artifactIPIPCountry:
		if source.format != formatTXT {
			return nil, fmt.Errorf("ipip country requires txt format, got %q", source.format)
		}
		return parseIPIPCountryZip(payload)
	default:
		return nil, fmt.Errorf(
			"unsupported GEO source %q/%q",
			source.provider,
			source.artifact,
		)
	}
}

func estimatedRangeCapacity(size uint64, averageLineBytes, maxCapacity int) int {
	if size == 0 || averageLineBytes <= 0 || maxCapacity <= 0 {
		return 0
	}
	capacity := size / uint64(averageLineBytes)
	if capacity > uint64(maxCapacity) {
		return maxCapacity
	}
	return int(capacity)
}

func parseIPToASNCombinedTSVAsn(payload []byte) ([]asnRange, error) {
	asnRanges := make([]asnRange, 0, estimatedRangeCapacity(uint64(len(payload)), 64, 1<<20))

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
	geoRanges := make([]geoRange, 0, estimatedRangeCapacity(uint64(len(payload)), 64, 1<<20))

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

	out := make([]asnRange, 0, estimatedRangeCapacity(uint64(len(payload)), 80, 1<<18))
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

func parseCAIDAPrefix2AS(payload []byte) ([]asnRange, error) {
	out := make([]asnRange, 0, estimatedRangeCapacity(uint64(len(payload)), 32, 1<<20))
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("caida prefix2as line %d: expected >= 3 columns", lineNo)
		}

		prefixLength, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("caida prefix2as line %d: invalid prefix length %q: %w", lineNo, fields[1], err)
		}
		prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", strings.TrimSpace(fields[0]), prefixLength))
		if err != nil {
			return nil, fmt.Errorf("caida prefix2as line %d: invalid prefix: %w", lineNo, err)
		}
		start, end := rangeFromPrefix(prefix.Masked())

		asn, err := parsePrimaryASN(fields[2])
		if err != nil {
			return nil, fmt.Errorf("caida prefix2as line %d: %w", lineNo, err)
		}
		if asn == 0 {
			continue
		}

		rec := asnRange{start: start, end: end, asn: asn}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("caida prefix2as line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan caida prefix2as payload: %w", err)
	}
	return out, nil
}

func parseDBIPCountryCSV(payload []byte) ([]geoRange, error) {
	reader := csv.NewReader(strings.NewReader(string(payload)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	out := make([]geoRange, 0, estimatedRangeCapacity(uint64(len(payload)), 64, 1<<18))
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

	out := make([]geoRange, 0, estimatedRangeCapacity(uint64(len(payload)), 128, 1<<18))
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

	out := make([]asnRange, 0)
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

	out := make([]geoRange, 0)
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

func parseMaxMindCountryCSVZip(payload []byte) ([]geoRange, error) {
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to open maxmind country csv zip: %w", err)
	}

	countryByID, err := parseMaxMindCountryLocations(archive)
	if err != nil {
		return nil, err
	}

	out := make([]geoRange, 0)
	for _, suffix := range []string{
		"GeoLite2-Country-Blocks-IPv4.csv",
		"GeoLite2-Country-Blocks-IPv6.csv",
	} {
		ranges, err := parseMaxMindCountryBlocks(archive, suffix, countryByID)
		if err != nil {
			return nil, err
		}
		out = append(out, ranges...)
	}
	return out, nil
}

func parseMaxMindCountryLocations(archive *zip.Reader) (map[string]string, error) {
	file, err := openZipEntrySuffix(archive, "GeoLite2-Country-Locations-en.csv")
	if err != nil {
		return nil, err
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	csvr := csv.NewReader(rc)
	csvr.FieldsPerRecord = -1
	header, err := csvr.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read maxmind locations header: %w", err)
	}
	idx := csvHeaderIndex(header)
	idIdx, ok := idx["geoname_id"]
	if !ok {
		return nil, fmt.Errorf("maxmind locations missing geoname_id column")
	}
	countryIdx, ok := idx["country_iso_code"]
	if !ok {
		return nil, fmt.Errorf("maxmind locations missing country_iso_code column")
	}

	out := map[string]string{}
	lineNo := 1
	for {
		row, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("maxmind locations line %d: %w", lineNo+1, err)
		}
		lineNo++
		if len(row) <= idIdx || len(row) <= countryIdx {
			continue
		}
		id := strings.TrimSpace(row[idIdx])
		country := normalizeCountry(row[countryIdx])
		if id != "" && country != "" {
			out[id] = country
		}
	}
	return out, nil
}

func parseMaxMindCountryBlocks(
	archive *zip.Reader,
	suffix string,
	countryByID map[string]string,
) ([]geoRange, error) {
	file, err := openZipEntrySuffix(archive, suffix)
	if err != nil {
		return nil, err
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	csvr := csv.NewReader(rc)
	csvr.FieldsPerRecord = -1
	header, err := csvr.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read maxmind blocks header %s: %w", suffix, err)
	}
	idx := csvHeaderIndex(header)
	networkIdx, ok := idx["network"]
	if !ok {
		return nil, fmt.Errorf("maxmind blocks %s missing network column", suffix)
	}
	idColumns := []string{"geoname_id", "registered_country_geoname_id", "represented_country_geoname_id"}

	out := make([]geoRange, 0, estimatedRangeCapacity(file.UncompressedSize64, 96, 1<<18))
	lineNo := 1
	for {
		row, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("maxmind blocks %s line %d: %w", suffix, lineNo+1, err)
		}
		lineNo++
		if len(row) <= networkIdx {
			continue
		}

		country := ""
		for _, column := range idColumns {
			columnIdx, ok := idx[column]
			if !ok || len(row) <= columnIdx {
				continue
			}
			if c := countryByID[strings.TrimSpace(row[columnIdx])]; c != "" {
				country = c
				break
			}
		}
		if country == "" {
			continue
		}

		rec, err := geoRangeFromToken(row[networkIdx], country)
		if err != nil {
			return nil, fmt.Errorf("maxmind blocks %s line %d: %w", suffix, lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseIP2LocationCountryZip(payload []byte) ([]geoRange, error) {
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to open ip2location country zip: %w", err)
	}
	file, err := openZipEntryBase(archive, "IP2LOCATION-LITE-DB1.CSV")
	if err != nil {
		return nil, err
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	csvr := csv.NewReader(rc)
	csvr.FieldsPerRecord = -1
	out := make([]geoRange, 0, estimatedRangeCapacity(file.UncompressedSize64, 64, 1<<18))
	lineNo := 0
	for {
		row, err := csvr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ip2location line %d: %w", lineNo+1, err)
		}
		lineNo++
		if len(row) < 3 {
			continue
		}
		country := normalizeCountry(row[2])
		if country == "" {
			continue
		}
		start, end, err := parseRangeEndpoints(row[0], row[1])
		if err != nil {
			return nil, fmt.Errorf("ip2location line %d: %w", lineNo, err)
		}
		rec := geoRange{start: start, end: end, country: country}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("ip2location line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

func parseIPDenyCountryTarGZ(payload []byte) ([]geoRange, error) {
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to open ipdeny tar.gz: %w", err)
	}
	defer gz.Close()

	out := make([]geoRange, 0)
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read ipdeny tar: %w", err)
		}
		name := path.Base(header.Name)
		zoneName, ok := strings.CutSuffix(strings.ToLower(name), ".zone")
		if header.Typeflag != tar.TypeReg || !ok {
			continue
		}
		country := normalizeCountry(zoneName)
		if country == "" {
			continue
		}
		ranges, err := parseCountryTokenLines(tr, country, "ipdeny "+header.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, ranges...)
	}
	return out, nil
}

func parseIPIPCountryZip(payload []byte) ([]geoRange, error) {
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to open ipip country zip: %w", err)
	}
	file, err := openZipEntryBase(archive, "country.txt")
	if err != nil {
		return nil, err
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	out := make([]geoRange, 0, estimatedRangeCapacity(file.UncompressedSize64, 48, 1<<18))
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		fields := strings.Fields(strings.ReplaceAll(scanner.Text(), "\r", ""))
		if len(fields) < 2 {
			continue
		}
		country := normalizeCountry(fields[len(fields)-1])
		if country == "" {
			continue
		}
		rec, err := geoRangeFromToken(fields[0], country)
		if err != nil {
			return nil, fmt.Errorf("ipip line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan ipip country payload: %w", err)
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

func parsePrimaryASN(raw string) (uint32, error) {
	value := strings.TrimSpace(strings.Trim(raw, "\"{}"))
	if value == "" {
		return 0, nil
	}
	for _, sep := range []string{"_", ","} {
		if idx := strings.Index(value, sep); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
	}
	return parseASN(value)
}

func csvHeaderIndex(header []string) map[string]int {
	out := make(map[string]int, len(header))
	for i, name := range header {
		out[strings.TrimSpace(name)] = i
	}
	return out
}

func openZipEntryBase(archive *zip.Reader, name string) (*zip.File, error) {
	for _, file := range archive.File {
		if path.Base(file.Name) == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("zip entry %q not found", name)
}

func openZipEntrySuffix(archive *zip.Reader, suffix string) (*zip.File, error) {
	for _, file := range archive.File {
		if strings.HasSuffix(file.Name, suffix) {
			return file, nil
		}
	}
	return nil, fmt.Errorf("zip entry with suffix %q not found", suffix)
}

func parseCountryTokenLines(r io.Reader, country, label string) ([]geoRange, error) {
	out := make([]geoRange, 0)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		token := strings.TrimSpace(scanner.Text())
		if token == "" || strings.HasPrefix(token, "#") {
			continue
		}
		fields := strings.Fields(token)
		if len(fields) == 0 {
			continue
		}
		rec, err := geoRangeFromToken(fields[0], country)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", label, lineNo, err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", label, err)
	}
	return out, nil
}

func geoRangeFromToken(raw, country string) (geoRange, error) {
	start, end, err := rangeFromToken(raw)
	if err != nil {
		return geoRange{}, err
	}
	rec := geoRange{start: start, end: end, country: country}
	if err := rec.validate(); err != nil {
		return geoRange{}, err
	}
	return rec, nil
}

func rangeFromToken(raw string) (netip.Addr, netip.Addr, error) {
	token := strings.TrimSpace(strings.Trim(raw, "\""))
	if token == "" {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("empty range token")
	}

	if strings.Contains(token, "/") {
		prefix, err := netip.ParsePrefix(token)
		if err != nil {
			return netip.Addr{}, netip.Addr{}, err
		}
		start, end := rangeFromPrefix(prefix.Masked())
		return start, end, nil
	}

	if strings.Contains(token, "-") {
		left, right, ok := strings.Cut(strings.ReplaceAll(token, " ", ""), "-")
		if !ok {
			return netip.Addr{}, netip.Addr{}, fmt.Errorf("invalid range token %q", raw)
		}
		return parseRangeEndpoints(left, right)
	}

	addr, err := parseIP(token)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	return addr, addr, nil
}

func rangeFromPrefix(prefix netip.Prefix) (netip.Addr, netip.Addr) {
	rng := netipx.RangeOfPrefix(prefix.Masked())
	return rng.From(), rng.To()
}
