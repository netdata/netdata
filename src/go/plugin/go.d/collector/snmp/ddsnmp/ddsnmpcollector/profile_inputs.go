// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// PreparedDeviceMetadata returns an owned copy of the last prepared metadata,
// without acquiring data. Like Collect, only the synchronous owner calls it.
func (c *Collector) PreparedDeviceMetadata() map[string]ddsnmp.MetaTag {
	metadata := make(map[string]ddsnmp.MetaTag)
	for _, state := range c.sortedProfileStates() {
		if state.initialized {
			ddsnmp.MergeMetaTags(metadata, state.cache.deviceMetadata)
		}
	}
	return metadata
}

// RefreshProfileInputs reacquires scalar profile inputs without changing table
// state. observedOIDs names system scalars authoritatively observed by the caller;
// their previous missing-OID decisions no longer apply. Failed profiles retain
// their last usable input snapshot while other profiles can install fresh inputs.
// Like Collect, only the synchronous owner calls it.
func (c *Collector) RefreshProfileInputs(observedOIDs []string) error {
	c.failures = ddsnmp.CollectionFailures{}
	c.requests = snmpRequestState{}
	for _, oid := range observedOIDs {
		oid = trimOID(oid)
		delete(c.missingOIDs, oid)
		delete(c.negative.eligible, oid)
		delete(c.negative.causes, oid)
	}

	var errs []error
	for ordinal, state := range c.sortedProfileStates() {
		staged := &profileState{
			profile:  state.profile,
			identity: state.identity,
		}
		prepared := c.newProfileCollection(staged, uint32(ordinal))
		err := c.prepareProfileInputsWithStrictness(prepared, true)
		c.failures.Processing.Preparation += prepared.metrics.Stats.Errors.Processing.Preparation
		if err != nil {
			err = fmt.Errorf("profile %q: %w", state.profile.SourceFile, err)
			recordCollectionFailure(&c.failures.Profiles, err, "prepare", "")
			errs = append(errs, err)
			continue
		}
		state.cache = staged.cache
		state.identity = staged.identity
		state.initialized = true
	}
	return errors.Join(errs...)
}

func getProfileInputValues(
	client snmputils.ScalarClient,
	oids []string,
	missingOIDs map[string]bool,
	stats *ddsnmp.CollectionStats,
	requireComplete bool,
) (map[string]gosnmp.SnmpPDU, error) {
	pdus, err := getSNMPValues(client, oids, missingOIDs, stats)
	if err != nil || !requireComplete {
		return pdus, err
	}
	for _, oid := range oids {
		oid = trimOID(oid)
		if _, ok := pdus[oid]; !ok && !missingOIDs[oid] {
			// Explicit absence is recorded by getSNMPValues. An unanswered OID
			// must not erase a previously usable input during refresh.
			stats.Errors.Processing.Preparation++
			return nil, snmputils.WithFailure(
				fmt.Errorf("SNMP GET omitted requested profile input OID %q", oid),
				"get",
				"processing",
			)
		}
	}
	return pdus, nil
}
