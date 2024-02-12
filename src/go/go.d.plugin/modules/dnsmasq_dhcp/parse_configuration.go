// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq_dhcp

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/netdata/go.d.plugin/pkg/iprange"
)

func (d *DnsmasqDHCP) parseDnsmasqDHCPConfiguration() ([]iprange.Range, []net.IP) {
	configs := findConfigurationFiles(d.ConfPath, d.ConfDir)

	dhcpRanges := d.getDHCPRanges(configs)
	dhcpHosts := d.getDHCPHosts(configs)

	return dhcpRanges, dhcpHosts
}

func (d *DnsmasqDHCP) getDHCPRanges(configs []*configFile) []iprange.Range {
	var dhcpRanges []iprange.Range
	var parsed string
	seen := make(map[string]bool)

	for _, conf := range configs {
		d.Debugf("looking in '%s'", conf.path)

		for _, value := range conf.get("dhcp-range") {
			d.Debugf("found dhcp-range '%s'", value)
			if parsed = parseDHCPRangeValue(value); parsed == "" || seen[parsed] {
				continue
			}
			seen[parsed] = true

			r, err := iprange.ParseRange(parsed)
			if r == nil || err != nil {
				d.Warningf("error on parsing dhcp-range '%s', skipping it", parsed)
				continue
			}

			d.Debugf("adding dhcp-range '%s'", parsed)
			dhcpRanges = append(dhcpRanges, r)
		}
	}

	// order: ipv4, ipv6
	sort.Slice(dhcpRanges, func(i, j int) bool { return dhcpRanges[i].Family() < dhcpRanges[j].Family() })

	return dhcpRanges
}

func (d *DnsmasqDHCP) getDHCPHosts(configs []*configFile) []net.IP {
	var dhcpHosts []net.IP
	seen := make(map[string]bool)
	var parsed string

	for _, conf := range configs {
		d.Debugf("looking in '%s'", conf.path)

		for _, value := range conf.get("dhcp-host") {
			d.Debugf("found dhcp-host '%s'", value)
			if parsed = parseDHCPHostValue(value); parsed == "" || seen[parsed] {
				continue
			}
			seen[parsed] = true

			v := net.ParseIP(parsed)
			if v == nil {
				d.Warningf("error on parsing dhcp-host '%s', skipping it", parsed)
				continue
			}

			d.Debugf("adding dhcp-host '%s'", parsed)
			dhcpHosts = append(dhcpHosts, v)
		}
	}
	return dhcpHosts
}

/*
Examples:
  - 192.168.0.50,192.168.0.150,12h
  - 192.168.0.50,192.168.0.150,255.255.255.0,12h
  - set:red,1.1.1.50,1.1.2.150, 255.255.252.0
  - 192.168.0.0,static
  - 1234::2,1234::500, 64, 12h
  - 1234::2,1234::500
  - 1234::2,1234::500, slaac
  - 1234::,ra-only
  - 1234::,ra-names
  - 1234::,ra-stateless
*/
var reDHCPRange = regexp.MustCompile(`([0-9a-f.:]+),([0-9a-f.:]+)`)

func parseDHCPRangeValue(s string) (r string) {
	if strings.Contains(s, "ra-stateless") {
		return
	}

	match := reDHCPRange.FindStringSubmatch(s)
	if match == nil {
		return
	}

	start, end := net.ParseIP(match[1]), net.ParseIP(match[2])
	if start == nil || end == nil {
		return
	}

	return fmt.Sprintf("%s-%s", start, end)
}

/*
Examples:
  - 11:22:33:44:55:66,192.168.0.60
  - 11:22:33:44:55:66,fred,192.168.0.60,45m
  - 11:22:33:44:55:66,12:34:56:78:90:12,192.168.0.60
  - bert,192.168.0.70,infinite
  - id:01:02:02:04,192.168.0.60
  - id:ff:00:00:00:00:00:02:00:00:02:c9:00:f4:52:14:03:00:28:05:81,192.168.0.61
  - id:marjorie,192.168.0.60
  - id:00:01:00:01:16:d2:83:fc:92:d4:19:e2:d8:b2, fred, [1234::5]
*/
var (
	reDHCPHostV4 = regexp.MustCompile(`(?:[0-9]{1,3}\.){3}[0-9]{1,3}`)
	reDHCPHostV6 = regexp.MustCompile(`\[([0-9a-f.:]+)]`)
)

func parseDHCPHostValue(s string) (r string) {
	if strings.Contains(s, "[") {
		return strings.Trim(reDHCPHostV6.FindString(s), "[]")
	}
	return reDHCPHostV4.FindString(s)
}

type (
	extension string

	extensions []extension

	configDir struct {
		path    string
		include extensions
		exclude extensions
	}
)

func (e extension) match(filename string) bool {
	return strings.HasSuffix(filename, string(e))
}

func (es extensions) match(filename string) bool {
	for _, e := range es {
		if e.match(filename) {
			return true
		}
	}
	return false
}

func parseConfDir(confDirStr string) configDir {
	// # Include all the files in a directory except those ending in .bak
	//#conf-dir=/etc/dnsmasq.d,.bak
	//# Include all files in a directory which end in .conf
	//#conf-dir=/etc/dnsmasq.d/,*.conf

	parts := strings.Split(confDirStr, ",")
	cd := configDir{path: parts[0]}

	for _, arg := range parts[1:] {
		arg = strings.TrimSpace(arg)
		if strings.HasPrefix(arg, "*") {
			cd.include = append(cd.include, extension(arg[1:]))
		} else {
			cd.exclude = append(cd.exclude, extension(arg))
		}
	}
	return cd
}

func (cd configDir) isValidFilename(filename string) bool {
	switch {
	default:
		return true
	case strings.HasPrefix(filename, "."):
	case strings.HasPrefix(filename, "~"):
	case strings.HasPrefix(filename, "#") && strings.HasSuffix(filename, "#"):
	}
	return false
}

func (cd configDir) match(filename string) bool {
	switch {
	default:
		return true
	case !cd.isValidFilename(filename):
	case len(cd.include) > 0 && !cd.include.match(filename):
	case cd.exclude.match(filename):
	}
	return false
}

func (cd configDir) findConfigs() ([]string, error) {
	fis, err := os.ReadDir(cd.path)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, fi := range fis {
		info, err := fi.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || !cd.match(fi.Name()) {
			continue
		}
		files = append(files, filepath.Join(cd.path, fi.Name()))
	}
	return files, nil
}

func openFile(filepath string) (f *os.File, err error) {
	defer func() {
		if err != nil && f != nil {
			_ = f.Close()
		}
	}()

	f, err = os.Open(filepath)
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("'%s' is not a regular file", filepath)
	}
	return f, nil
}

type (
	configOption struct {
		key, value string
	}

	configFile struct {
		path    string
		options []configOption
	}
)

func (cf *configFile) get(name string) []string {
	var options []string
	for _, o := range cf.options {
		if o.key != name {
			continue
		}
		options = append(options, o.value)
	}
	return options
}

func parseConfFile(filename string) (*configFile, error) {
	f, err := openFile(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	cf := configFile{path: filename}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.Contains(line, "=") {
			continue
		}

		line = strings.ReplaceAll(line, " ", "")
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			continue
		}

		cf.options = append(cf.options, configOption{key: parts[0], value: parts[1]})
	}
	return &cf, nil
}

type ConfigFinder struct {
	entryConfig    string
	entryDir       string
	visitedConfigs map[string]bool
	visitedDirs    map[string]bool
}

func (f *ConfigFinder) find() []*configFile {
	f.visitedConfigs = make(map[string]bool)
	f.visitedDirs = make(map[string]bool)

	configs := f.recursiveFind(f.entryConfig)

	for _, file := range f.entryDirConfigs() {
		configs = append(configs, f.recursiveFind(file)...)
	}
	return configs
}

func (f *ConfigFinder) entryDirConfigs() []string {
	if f.entryDir == "" {
		return nil
	}
	files, err := parseConfDir(f.entryDir).findConfigs()
	if err != nil {
		return nil
	}
	return files
}

func (f *ConfigFinder) recursiveFind(filename string) (configs []*configFile) {
	if f.visitedConfigs[filename] {
		return nil
	}

	config, err := parseConfFile(filename)
	if err != nil {
		return nil
	}

	files, dirs := config.get("conf-file"), config.get("conf-dir")

	f.visitedConfigs[filename] = true
	configs = append(configs, config)

	for _, file := range files {
		configs = append(configs, f.recursiveFind(file)...)
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		d := parseConfDir(dir)

		if f.visitedDirs[d.path] {
			continue
		}
		f.visitedDirs[d.path] = true

		files, err = d.findConfigs()
		if err != nil {
			continue
		}

		for _, file := range files {
			configs = append(configs, f.recursiveFind(file)...)
		}
	}
	return configs
}

func findConfigurationFiles(entryConfig string, entryDir string) []*configFile {
	cf := ConfigFinder{
		entryConfig: entryConfig,
		entryDir:    entryDir,
	}
	return cf.find()
}
