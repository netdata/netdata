# Topology Library Phase 2 Gap Report

- Generated at (UTC): `2026-02-21T03:05:05Z`
- Overall status: `pass`
- Scenario parity: `46/46` passed
- Assertion parity: `3067/3067` mapped

## What Matches Enlinkd (In Scope)

- `lldp`: `6/6` checks passed (status: `pass`).
- `cdp`: `2/2` checks passed (status: `pass`).
- `bridge_fdb_arp`: `4/4` checks passed (status: `pass`).
- `updater`: `2/2` checks passed (status: `pass`).
- In-scope assertion coverage: `1240/1240` ported, `0` not-applicable-approved, `0` unmapped.

## Runtime Quality Checks

- Reverse pair quality: `5/5` checks passed (status: `pass`).
- Identity merge quality: `1/1` checks passed (status: `pass`).

## Intentionally Deferred Gaps

- none

## Command Evidence

- `go test -json ./pkg/topology/engine -run ^(TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses|TestMatchLLDPLinksEnlinkdPassOrder_Precedence)$ -count=1`
- `go test -json ./pkg/topology/engine -run ^(TestMatchCDPLinksEnlinkdPassOrder_DefaultAndParsedTarget|TestMatchCDPLinksEnlinkdPassOrder_SkipsSelfTarget)$ -count=1`
- `go test -json ./pkg/topology/engine -run ^(TestBuildL2ResultFromObservations_FDBAttachments|TestBuildL2ResultFromObservations_FDBBridgeDomainFallbackToBridgePort|TestBuildL2ResultFromObservations_FDBDropsDuplicateMACAcrossPorts|TestBuildL2ResultFromObservations_FDBSkipsSelfAndNonLearned)$ -count=1`
- `go test -json ./pkg/topology/engine -run ^(TestBuildL2ResultFromObservations_AnnotatesPairMetadata)$ -count=1`
- `go test -json ./pkg/topology/engine -run ^(TestToTopologyData_MergesPairedAdjacenciesIntoBidirectionalLink)$ -count=1`
- `go test -json ./plugin/go.d/collector/snmp -run ^(TestTopologyCache_CdpSnapshot|TestTopologyCache_CdpSnapshotHexAddress|TestTopologyCache_CdpSnapshotRawAddressWithoutIP|TestTopologyCache_LldpSnapshot|TestTopologyCache_SnapshotBidirectionalPairMetadata)$ -count=1`
- `go test -json ./plugin/go.d/collector/snmp -run ^(TestTopologyCache_SnapshotMergesRemoteIdentityAcrossProtocols)$ -count=1`
