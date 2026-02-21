# Enlinkd topology parity test data attribution

Source: https://github.com/opennms/opennms
License: AGPL-3.0-or-later
Reference commit: `057ded59`

These fixture files are reduced subsets derived from OpenNMS Enlinkd test resources:

- `features/enlinkd/tests/src/test/resources/linkd/nms8003/NMM-R1.snmpwalk.txt`
- `features/enlinkd/tests/src/test/resources/linkd/nms8003/NMM-R2.snmpwalk.txt`
- `features/enlinkd/tests/src/test/resources/linkd/nms8003/NMM-R3.snmpwalk.txt`
- `features/enlinkd/tests/src/test/resources/linkd/nms8003/NMM-SW1.snmpwalk.txt`
- `features/enlinkd/tests/src/test/resources/linkd/nms8003/NMM-SW2.snmpwalk.txt`

Stored subset location:

- `src/go/pkg/topology/engine/testdata/enlinkd/nms8003/fixtures/`

Subset criteria:

- LLDP MIB OIDs (`1.0.8802.1.1.2.1.*`)
- core system identity OIDs (`1.3.6.1.2.1.1.2.0`, `1.3.6.1.2.1.1.5.0`)
- interface naming OIDs (`1.3.6.1.2.1.2.2.1.2.*`, `1.3.6.1.2.1.31.1.1.1.1.*`)

These files are used by the topology engine parity fixture ingestion and golden-cache tests.
