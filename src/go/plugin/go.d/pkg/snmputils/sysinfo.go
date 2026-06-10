// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

var log = logger.New().With("component", "snmp/sysinfo")

const (
	RootOidMibSystem = "1.3.6.1.2.1.1"
	OidSysDescr      = "1.3.6.1.2.1.1.1.0"
	OidSysObject     = "1.3.6.1.2.1.1.2.0"
	OidSysContact    = "1.3.6.1.2.1.1.4.0"
	OidSysName       = "1.3.6.1.2.1.1.5.0"
	OidSysLocation   = "1.3.6.1.2.1.1.6.0"
)

type SysInfo struct {
	SysObjectID string `json:"-"`

	Descr    string `json:"description"`
	Contact  string `json:"contact"`
	Name     string `json:"name"`
	Location string `json:"location"`

	Organization string `json:"organization"`
	Vendor       string `json:"vendor"`
	Category     string `json:"category"`
	Model        string `json:"model"`
}

func GetSysInfo(client gosnmp.Handler) (*SysInfo, error) {
	pdus, err := client.WalkAll(RootOidMibSystem)
	if err != nil {
		return nil, err
	}

	si := &SysInfo{
		Name:         "unknown",
		Organization: "Unknown",
	}

	loadOverrides()

	for _, pdu := range pdus {
		oid := strings.TrimPrefix(pdu.Name, ".")

		switch oid {
		case OidSysDescr:
			si.Descr, err = PduToString(pdu)
			si.Descr = valueSanitizer.Replace(si.Descr)
		case OidSysObject:
			var sysObj string
			if sysObj, err = PduToString(pdu); err == nil {
				si.SysObjectID = sysObj
			}
		case OidSysContact:
			si.Contact, err = PduToString(pdu)
			si.Contact = valueSanitizer.Replace(si.Contact)
		case OidSysName:
			si.Name, err = PduToString(pdu)
			si.Name = valueSanitizer.Replace(si.Name)
		case OidSysLocation:
			si.Location, err = PduToString(pdu)
			si.Location = valueSanitizer.Replace(si.Location)
		}
		if err != nil {
			return nil, fmt.Errorf("OID '%s': %v", pdu.Name, err)
		}
	}

	updateMetadata(si)

	return si, nil
}

var valueSanitizer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
	"\"", "",
	"`", "",
	"\\", "",
)

// updateMetadata enriches a SysInfo struct with metadata based on its SysObjectID.
// It populates the Organization, Vendor, Category, and Model fields.
func updateMetadata(si *SysInfo) {
	if si == nil || si.SysObjectID == "" {
		return
	}

	rawOrg := lookupEnterpriseNumber(si.SysObjectID)

	var finalCategory string
	var finalModel string
	var finalVendor string

	if overridesData != nil {
		// Apply specific OID overrides for category and model.
		if override, found := overridesData.SysObjectIDs.OIDOverrides[si.SysObjectID]; found {
			if override.Category != "" {
				finalCategory = override.Category
			}
			if override.Model != "" {
				finalModel = override.Model
			}
		}

		// Normalize the category *after* applying the specific override.
		if normalized, found := overridesData.SysObjectIDs.CategoryMap[finalCategory]; found {
			finalCategory = normalized
		}

		// Map the raw organization name to a standardized vendor name.
		if vendor, found := overridesData.EnterpriseNumbers.OrgToVendor[rawOrg]; found {
			finalVendor = vendor
		}
	}

	si.Organization = valueSanitizer.Replace(rawOrg)
	si.Category = finalCategory
	si.Model = finalModel
	si.Vendor = finalVendor
}

type enterpriseNumbersCache struct {
	once   sync.Once
	values map[string]string
	err    error
}

var (
	enterpriseNumbers             enterpriseNumbersCache
	enterpriseNumbersPathOverride string
)

func lookupEnterpriseNumber(sysObject string) string {
	const rootOidIanaPEN = "1.3.6.1.4.1"
	v, ok := strings.CutPrefix(sysObject, rootOidIanaPEN+".") // .1.3.6.1.4.1.14988.1 => 14988.1
	if !ok {
		return ""
	}
	num, _, ok := strings.Cut(v, ".")
	if !ok {
		return ""
	}

	mapping, err := enterpriseNumbersMapping()
	if err != nil {
		return ""
	}
	return mapping[num]
}

func enterpriseNumbersMapping() (map[string]string, error) {
	enterpriseNumbers.once.Do(func() {
		enterpriseNumbers.values, enterpriseNumbers.err = loadEnterpriseNumbers(enterpriseNumbersFilePath())
		if enterpriseNumbers.err != nil {
			log.Debugf("cannot load IANA PEN registry: %v", enterpriseNumbers.err)
		}
	})
	return enterpriseNumbers.values, enterpriseNumbers.err
}

func enterpriseNumbersFilePath() string {
	if path := strings.TrimSpace(enterpriseNumbersPathOverride); path != "" {
		return path
	}
	if dir := strings.TrimSpace(buildinfo.StockConfigDir); dir != "" {
		return filepath.Join(dir, "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt")
	}
	for _, candidate := range enterpriseNumbersFileCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return filepath.Join("/usr/lib/netdata/conf.d", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt")
}

func enterpriseNumbersFileCandidates() []string {
	candidates := []string{
		filepath.Join("..", "..", "config", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt"),
		filepath.Join("plugin", "go.d", "config", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt"),
		filepath.Join("src", "go", "plugin", "go.d", "config", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt"),
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		sourceCandidate := filepath.Join(filepath.Dir(file), "..", "..", "config", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt")
		candidates = append([]string{sourceCandidate}, candidates...)
	}
	return candidates
}

func loadEnterpriseNumbers(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	mapping, err := parseEnterpriseNumbers(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(mapping) == 0 {
		return nil, fmt.Errorf("%s: empty IANA PEN registry", path)
	}
	return mapping, nil
}

func parseEnterpriseNumbers(r io.Reader) (map[string]string, error) {
	mapping := make(map[string]string, 65000)
	var id string

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if _, err := strconv.Atoi(line); err == nil {
			if id == "" {
				id = line
				if _, ok := mapping[id]; ok {
					return nil, fmt.Errorf("duplicate entry number: %s", line)
				}
			}
			continue
		}
		if id != "" {
			if line == "---none---" || line == "Reserved" {
				id = ""
				continue
			}
			mapping[id] = line
			id = ""
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return mapping, nil
}

func PduToString(pdu gosnmp.SnmpPDU) (string, error) {
	switch pdu.Type {
	case gosnmp.OctetString:
		// TODO: this isn't reliable (e.g. physAddress we need hex.EncodeToString())
		bs, ok := pdu.Value.([]byte)
		if !ok {
			return "", fmt.Errorf("OctetString is not a []byte but %T", pdu.Value)
		}
		return strings.ToValidUTF8(string(bs), "�"), nil
	case gosnmp.Counter32, gosnmp.Counter64, gosnmp.Integer, gosnmp.Gauge32:
		return gosnmp.ToBigInt(pdu.Value).String(), nil
	case gosnmp.ObjectIdentifier:
		v, ok := pdu.Value.(string)
		if !ok {
			return "", fmt.Errorf("ObjectIdentifier is not a string but %T", pdu.Value)
		}
		return strings.TrimPrefix(v, "."), nil
	default:
		return "", fmt.Errorf("unsupported type: '%v'", pdu.Type)
	}
}
