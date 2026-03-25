// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"net/netip"
	"strconv"
	"strings"
)

func loadRanges(cfg config, dl *downloader) ([]asnRange, []countryRange, error) {
	source := cfg.source
	switch source.provider {
	case providerIPToASN:
		spec := source.combined
		if spec.format == "" {
			spec.format = formatIPToASNCombinedTSV
		}
		if spec.compression == "" {
			spec.compression = compressionAuto
		}
		content, err := dl.readDataset(spec)
		if err != nil {
			return nil, nil, err
		}
		return parseDataset(spec.format, content)
	case providerDBIP:
		asnSpec := source.asn
		if asnSpec.format == "" {
			asnSpec.format = formatDBIPAsnCSV
		}
		if asnSpec.compression == "" {
			asnSpec.compression = compressionAuto
		}
		countrySpec := source.country
		if countrySpec.format == "" {
			countrySpec.format = formatDBIPCountryCSV
		}
		if countrySpec.compression == "" {
			countrySpec.compression = compressionAuto
		}
		asnContent, err := dl.readDataset(asnSpec)
		if err != nil {
			return nil, nil, err
		}
		countryContent, err := dl.readDataset(countrySpec)
		if err != nil {
			return nil, nil, err
		}
		asnRanges, _, err := parseDataset(asnSpec.format, asnContent)
		if err != nil {
			return nil, nil, err
		}
		_, countryRanges, err := parseDataset(countrySpec.format, countryContent)
		if err != nil {
			return nil, nil, err
		}
		return asnRanges, countryRanges, nil
	case providerCustom:
		var outASN []asnRange
		var outCountry []countryRange
		if source.combined.path != "" || source.combined.url != "" {
			spec := source.combined
			if spec.compression == "" {
				spec.compression = compressionAuto
			}
			content, err := dl.readDataset(spec)
			if err != nil {
				return nil, nil, err
			}
			asnRanges, countryRanges, err := parseDataset(spec.format, content)
			if err != nil {
				return nil, nil, err
			}
			outASN = append(outASN, asnRanges...)
			outCountry = append(outCountry, countryRanges...)
		}
		if source.asn.path != "" || source.asn.url != "" {
			spec := source.asn
			if spec.compression == "" {
				spec.compression = compressionAuto
			}
			content, err := dl.readDataset(spec)
			if err != nil {
				return nil, nil, err
			}
			asnRanges, _, err := parseDataset(spec.format, content)
			if err != nil {
				return nil, nil, err
			}
			outASN = append(outASN, asnRanges...)
		}
		if source.country.path != "" || source.country.url != "" {
			spec := source.country
			if spec.compression == "" {
				spec.compression = compressionAuto
			}
			content, err := dl.readDataset(spec)
			if err != nil {
				return nil, nil, err
			}
			_, countryRanges, err := parseDataset(spec.format, content)
			if err != nil {
				return nil, nil, err
			}
			outCountry = append(outCountry, countryRanges...)
		}
		return outASN, outCountry, nil
	default:
		return nil, nil, fmt.Errorf("unsupported provider %q", source.provider)
	}
}

func parseDataset(format string, payload []byte) ([]asnRange, []countryRange, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case formatIPToASNCombinedTSV:
		return parseIPToASNCombinedTSV(payload)
	case formatDBIPAsnCSV:
		asnRanges, err := parseDBIPAsnCSV(payload)
		return asnRanges, nil, err
	case formatDBIPCountryCSV:
		countryRanges, err := parseDBIPCountryCSV(payload)
		return nil, countryRanges, err
	default:
		return nil, nil, fmt.Errorf("unsupported dataset format %q", format)
	}
}

func parseIPToASNCombinedTSV(payload []byte) ([]asnRange, []countryRange, error) {
	asnRanges := make([]asnRange, 0, 1<<20)
	countryRanges := make([]countryRange, 0, 1<<20)

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
			return nil, nil, fmt.Errorf("iptoasn line %d: expected at least 5 columns", lineNo)
		}
		start, end, err := parseRangeEndpoints(parts[0], parts[1])
		if err != nil {
			return nil, nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		asn, err := parseASN(parts[2])
		if err != nil {
			return nil, nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		country := normalizeCountry(parts[3])
		org := strings.TrimSpace(parts[4])

		asnRange := asnRange{start: start, end: end, asn: asn, org: org}
		if err := asnRange.validate(); err != nil {
			return nil, nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
		}
		asnRanges = append(asnRanges, asnRange)

		if country != "" {
			countryRange := countryRange{start: start, end: end, country: country}
			if err := countryRange.validate(); err != nil {
				return nil, nil, fmt.Errorf("iptoasn line %d: %w", lineNo, err)
			}
			countryRanges = append(countryRanges, countryRange)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to scan iptoasn payload: %w", err)
	}
	return asnRanges, countryRanges, nil
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

func parseDBIPCountryCSV(payload []byte) ([]countryRange, error) {
	reader := csv.NewReader(strings.NewReader(string(payload)))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	out := make([]countryRange, 0, 1<<18)
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
		rec := countryRange{start: start, end: end, country: country}
		if err := rec.validate(); err != nil {
			return nil, fmt.Errorf("dbip country line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	return out, nil
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

	// Some providers encode IPv4 as decimal integers.
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
