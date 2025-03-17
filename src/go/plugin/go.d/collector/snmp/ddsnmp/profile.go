// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

var once sync.Once
var ddProfiles []*Profile

func Find(sysObjId string) []*Profile {
	once.Do(func() {
		dir := os.Getenv("NETDATA_STOCK_CONFIG_DIR")
		if dir == "" {
			dir = filepath.Join(executable.Directory, "../../../../usr/lib/netdata/conf.d/go.d/snmp.profiles/default/")
		}

		profiles, err := load(dir)
		if err != nil {
			log.Errorf("failed to load dd snmp profiles: %v", err)
			return
		}
		if len(profiles) == 0 {
			log.Warningf("no dd snmp profiles found in '%s'", dir)
			return
		}

		ddProfiles = profiles
	})

	var profiles []*Profile

	for _, prof := range ddProfiles {
		for _, id := range prof.Definition.SysObjectIDs {
			m, err := matcher.NewRegExpMatcher(id)
			if err != nil {
				log.Warningf("failed to compile regular expression from '%s': %v", id, err)
				continue
			}
			if m.MatchString(sysObjId) {
				profiles = append(profiles, prof.clone())
			}
		}
	}

	return profiles
}

type Profile struct {
	SourceFile string                               `yaml:"-"`
	Definition *profiledefinition.ProfileDefinition `yaml:",inline"`
}

func (p *Profile) clone() *Profile {
	return &Profile{
		SourceFile: p.SourceFile,
		Definition: p.Definition.Clone(),
	}
}

func (p *Profile) merge(base *Profile) {
	p.Definition.Metrics = append(p.Definition.Metrics, base.Definition.Metrics...)
	p.Definition.MetricTags = append(p.Definition.MetricTags, base.Definition.MetricTags...)
	p.Definition.StaticTags = append(p.Definition.StaticTags, base.Definition.StaticTags...)

	if p.Definition.Metadata == nil {
		p.Definition.Metadata = make(profiledefinition.MetadataConfig)
	}

	for resName, baseRes := range base.Definition.Metadata {
		targetRes, exists := p.Definition.Metadata[resName]
		if !exists {
			targetRes = profiledefinition.NewMetadataResourceConfig()
		}

		targetRes.IDTags = append(targetRes.IDTags, baseRes.IDTags...)

		if targetRes.Fields == nil && len(baseRes.Fields) > 0 {
			targetRes.Fields = make(map[string]profiledefinition.MetadataField, len(baseRes.Fields))
		}

		for field, symbol := range baseRes.Fields {
			if _, ok := targetRes.Fields[field]; !ok {
				targetRes.Fields[field] = symbol
			}
		}

		p.Definition.Metadata[resName] = targetRes
	}
}

func (p *Profile) validate() error {
	profiledefinition.NormalizeMetrics(p.Definition.Metrics)

	errs := profiledefinition.ValidateEnrichMetadata(p.Definition.Metadata)
	errs = append(errs, profiledefinition.ValidateEnrichMetrics(p.Definition.Metrics)...)
	errs = append(errs, profiledefinition.ValidateEnrichMetricTags(p.Definition.MetricTags)...)
	if len(errs) > 0 {
		errList := make([]error, 0, len(errs))
		for _, s := range errs {
			errList = append(errList, errors.New(s))
		}
		return errors.Join(errList...)
	}

	return nil
}
