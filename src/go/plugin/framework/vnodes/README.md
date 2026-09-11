# Configured virtual nodes

This document is the maintainer contract for configured vnode acquisition and attachment.
Operator configuration is documented in [node identities](../../../../../docs/learn/node-identities.md).

Place in the documentation set: the collectors-go-design skill cites Ownership and Collector attachment as the
existing framework boundary for vnode design work. This maintainer document is not a Learn page.

## Ownership

- `Config` is authored configuration, including acquisition credentials. `VirtualNode` is public host metadata.
  Credentials MUST NOT enter public snapshots, host definitions, acquisition diagnostics or whole-record logs.
- `VNodeConfiguration` owns authored records, raw acquired metadata, resolved snapshots and GUID/hostname indexes.
  Configured overrides are applied to raw metadata on every update, so removing an override needs no acquisition.
- Each run owns independent acquisition workers. The go.d entrypoint injects the concrete SNMP adapter; generic
  composition MUST NOT import collectors. Plugins without the adapter omit the mode from their form and filter
  unsupported file definitions before set-wide uniqueness checks.
- Acquisition does not register a connection in SNMP's `DeviceStore`. Only a metrics job owns that connection.
  A named SNMP job combines its own connection, profiles and system observations with the configured vnode identity.

## Acquisition and publication

- The one-shot adapter returns raw metadata plus an optional enrichment error. Nil metadata means no usable identity.
  System identity requires a nonempty usable sysObjectID, sysName or sysDescr. Profile enrichment uses automatic
  matching and scalar metadata reads only; metric profile coverage is not a readiness condition.
- First usable metadata makes attachment possible, including partial system identity with failed enrichment.
  A failed refresh retains the entire last usable snapshot. Complete success replaces acquired metadata.
  Rejected metadata, including hostname collisions, produces a safe diagnostic after the fallback state commits.
- Workers perform network I/O outside graph, configuration and output locks. Results commit through existing vnode
  transaction lanes. A record-pointer comparison fences prepared mutations; an acquisition token fences old requests
  after credential changes or remove/re-add. Label/hostname overrides preserve the acquisition token.
- Retry and refresh intervals are internal fixed values (10 seconds and one hour). Each attempt has fresh transport
  and metadata-reader state. There is no persistent acquired cache; a new run must acquire identity again.
- Credential changes mark status accepted while pending, even when last-good metadata remains available. Completion
  reports running or failed. DynCfg test validates authored configuration without network access; get returns authored
  configuration. The result path never substitutes acquired metadata into authored credentials/configuration.
- File and DynCfg input discard inactive authentication fields before validation and storage in Go. The selected version
  and security level determine active credentials; those remain strictly validated. Acquisition comparison uses the
  same normalization, so changes to inactive fields cannot restart acquisition. Source files and the C DynCfg layer's
  saved original request are unchanged; replay is normalized again at the Go boundary.
- Removal cancels the vnode's worker; run shutdown cancels and joins all workers. Late results cannot publish into a
  retired run or incarnation.
- Acquisition updates the configured host authority but MUST NOT announce a host by itself. Ordinary job output
  publishes it under the existing [host ownership contract](../hostoutput/README.md#metadata-ownership).

## Collector attachment

V1 and V2 runtimes supply `collectorapi.ConfiguredVnodeConsumer` with an owned public snapshot before Init/Check and
synchronously before the next Collect when its revision changes. The callback MUST be cheap and MUST NOT perform
network I/O. It is separate from the collector's `VirtualNode()` generated-identity capability.

SNMP uses `vnode` for a string reference and `local_vnode` for job-owned generation settings. Legacy inline `vnode`
objects decode to `local_vnode`; configuration output uses this canonical shape for stable form editing. Supplying
both local object spellings is rejected as ambiguous. A string suppresses collector-generated vnodes even
when `create_vnode` is true. The collector publishes canonical identity into its job-owned DeviceStore registration;
the provider's credentials are never substituted. The configured vnode owns host labels; job labels remain chart labels.

## Validation

- `config_test.go`, discovery vnode tests and composition acquisition tests cover authored/resolved separation,
  current override precedence, partial readiness, retention, mode-aware edits, uniqueness and generation fencing.
- `plugin/go.d/vnode/snmp_test.go` covers real UDP, missing v1 OIDs, enrichment failure and blocked-read cancellation.
- Composition's `snmp_vnode_integration_test.go` loads vnode YAML, constructs actual Apache and SNMP jobs, and checks
  shared identity, independent credentials, acquisition without metrics jobs and cold restarts with unavailable SNMP.
- V1 configured-consumer tests cover owned snapshots and revision updates; existing V1/V2 host publication suites
  remain the authority for output ordering, scope routing and configured-host precedence.
