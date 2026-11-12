// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func metadataTestProfile(sourceFile, oid string) *ddsnmp.Profile {
	profile := createTestProfile(sourceFile, nil)
	profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
		"device": {Fields: map[string]ddprofiledefinition.MetadataField{
			"name": {Symbol: ddprofiledefinition.SymbolConfig{OID: oid}},
		}},
	}
	return profile
}

func TestPreparedDeviceMetadataFollowsProfilePreparation(t *testing.T) {
	const oid = "1.2.3.0"
	getErr := errors.New("request failed")
	handler := &sourceTestHandler{
		get: func([]string) (*gosnmp.SnmpPacket, error) { return nil, getErr },
	}
	collector := New(Config{
		SnmpClient: handler,
		Profiles:   []*ddsnmp.Profile{metadataTestProfile("device.yaml", oid)},
		Log:        logger.New(),
	})

	assert.Zero(t, collector.ProfileInputRevision())
	assert.Empty(t, collector.PreparedDeviceMetadata())

	_, err := collector.Collect()
	require.ErrorIs(t, err, getErr)
	assert.Zero(t, collector.ProfileInputRevision(), "failed preparation publishes nothing")
	assert.Empty(t, collector.PreparedDeviceMetadata())

	handler.get = func([]string) (*gosnmp.SnmpPacket, error) {
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createStringPDU(oid, "device-a")}}, nil
	}
	_, err = collector.Collect()
	require.NoError(t, err)
	want := map[string]ddsnmp.MetaTag{"name": {Value: "device-a"}}
	assert.EqualValues(t, 1, collector.ProfileInputRevision())
	assert.Equal(t, want, collector.PreparedDeviceMetadata())

	gets := handler.gets
	_, err = collector.Collect()
	require.NoError(t, err)
	assert.Equal(t, gets, handler.gets, "prepared inputs are not acquired again")
	assert.EqualValues(t, 1, collector.ProfileInputRevision())
}

func TestCachedInputsExcludeUnpreparedProfiles(t *testing.T) {
	const tagOID, metadataOID = "1.2.3.1.0", "1.2.3.2.0"
	profile := metadataTestProfile("device.yaml", metadataOID)
	profile.Definition.MetricTags = []ddprofiledefinition.GlobalMetricTagConfig{{
		MetricTagConfig: ddprofiledefinition.MetricTagConfig{
			Tag:    "site",
			Symbol: ddprofiledefinition.SymbolConfigCompat{OID: tagOID, Name: "site"},
		},
	}}
	failMetadata := true
	handler := &sourceTestHandler{
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			if failMetadata && oids[0] == metadataOID {
				return nil, errors.New("request failed")
			}
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createStringPDU(oids[0], "value")}}, nil
		},
	}
	collector := New(Config{
		SnmpClient: new(SourceRecorder).Wrap(handler),
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})

	_, err := collector.Collect()
	require.Error(t, err)
	assert.Empty(t, collector.CachedInputs(), "tags acquired by a failed preparation are not a cache")

	failMetadata = false
	_, err = collector.Collect()
	require.NoError(t, err)
	var kinds []string
	for _, input := range collector.CachedInputs() {
		kinds = append(kinds, input.Kind)
	}
	assert.Equal(t, []string{"profile_metadata", "profile_tags"}, kinds)
}

func TestProfileInputMetadataFailureClassification(t *testing.T) {
	handler := &sourceTestHandler{
		get: func([]string) (*gosnmp.SnmpPacket, error) { return nil, gosnmp.ErrWrongDigest },
	}
	collector := New(Config{
		SnmpClient: handler,
		Profiles:   []*ddsnmp.Profile{metadataTestProfile("device.yaml", "1.2.3.0")},
		Log:        logger.New(),
	})

	_, err := collector.Collect()
	require.ErrorIs(t, err, gosnmp.ErrWrongDigest)
	failure := snmputils.ClassifyFailure(err)
	assert.Equal(t, "metadata", failure.Operation)
	assert.Equal(t, "wrong_digest", failure.Reason)
}
