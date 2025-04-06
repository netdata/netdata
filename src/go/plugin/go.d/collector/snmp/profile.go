package snmp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gosnmp/gosnmp"
	"gopkg.in/yaml.v3"
)

func (s *SysObjectIDs) UnmarshalYAML(unmarshal func(any) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*s = []string{single}
		return nil
	}

	var multiple []string
	if err := unmarshal(&multiple); err == nil {
		*s = multiple
		return nil
	}

	return fmt.Errorf("invalid sysobjectid format")
}

func (c *Collector) parseMetricsFromProfiles(matchingProfiles []*Profile) (map[string]processedMetric, error) {
	metricMap := map[string]processedMetric{}
	for _, profile := range matchingProfiles {
		results, err := parseMetrics(profile.Metrics)
		if err != nil {
			return nil, err
		}

		for _, oid := range results.oids {
			response, err := c.snmpClient.Get([]string{oid})
			if err != nil {
				return nil, err
			}
			if (response != &gosnmp.SnmpPacket{}) {
				for _, metric := range results.parsed_metrics {
					switch s := metric.(type) {
					case parsedSymbolMetric:
						// find a matching metric
						if s.baseoid == oid {
							metricName := s.name
							metricType := response.Variables[0].Type
							metricValue := response.Variables[0].Value

							metricMap[oid] = processedMetric{oid: oid, name: metricName, value: metricValue, metric_type: metricType}
						}
					}
				}

			}
		}

		for _, oid := range results.next_oids {
			if len(oid) < 1 {
				continue
			}
			if tableRows, err := c.walkOIDTree(oid); err != nil {
				log.Fatalf("Error walking OID tree: %v, oid %s", err, oid)
			} else {
				for _, metric := range results.parsed_metrics {
					switch s := metric.(type) {
					case parsedTableMetric:
						// find a matching metric
						if s.rowOID == oid {
							for key, value := range tableRows {
								value.name = s.name
								value.tableName = s.tableName
								tableRows[key] = value
							}
							metricMap = mergeProcessedMetricMaps(metricMap, tableRows)
						}
					}
				}
			}

		}

	}
	return metricMap, nil
}

func (s *Symbol) UnmarshalYAML(node *yaml.Node) error {
	// If scalar node, assume the value is the name.
	if node.Kind == yaml.ScalarNode {
		s.Name = node.Value
		return nil
	}

	// Otherwise, decode normally
	type plainSymbol Symbol
	var ps plainSymbol
	if err := node.Decode(&ps); err != nil {
		return err
	}
	*s = Symbol(ps)
	return nil
}

func LoadAllProfiles(profileDir string) (map[string]*Profile, error) {
	profiles := make(map[string]*Profile)
	err := filepath.Walk(profileDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), ".yaml") {
			profile, err := LoadYAML(path, profileDir)
			if err == nil {
				profiles[path] = profile
			} else {
				log.Printf("Skipping invalid YAML: %s (%v)\n", path, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return profiles, nil
}

func LoadYAML(filename string, basePath string) (*Profile, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var profile Profile
	err = yaml.Unmarshal(data, &profile)
	if err != nil {
		return nil, err
	}

	// If the profile extends other files, load and merge them
	for _, parentFile := range profile.Extends {
		parentProfile, err := LoadYAML(filepath.Join(basePath, parentFile), basePath)
		if err != nil {
			return nil, err
		}
		MergeProfiles(&profile, parentProfile)
	}

	return &profile, nil
}

// Merge two profiles, giving priority to the child profile
func MergeProfiles(child, parent *Profile) {
	if child.Metadata.Device.Fields == nil {
		child.Metadata.Device.Fields = make(map[string]Symbol)
	}

	for key, value := range parent.Metadata.Device.Fields {
		if _, exists := child.Metadata.Device.Fields[key]; !exists {
			child.Metadata.Device.Fields[key] = value
		}
	}
	child.Metrics = append(parent.Metrics, child.Metrics...)
}

// Find the matching profile based on sysObjectID
func FindMatchingProfiles(profiles map[string]*Profile, deviceOID string) []*Profile {
	var matchedProfiles []*Profile

	for _, profile := range profiles {
		for _, oidPattern := range profile.SysObjectID {
			if strings.HasPrefix(deviceOID, strings.Split(oidPattern, "*")[0]) {
				matchedProfiles = append(matchedProfiles, profile)
				break
			}
		}
	}

	return matchedProfiles
}
