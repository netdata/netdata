# Ceph Dashboard contract fixtures

These fixtures are synthetic, sanitized contract documents derived from read-only Dashboard validation and then
cross-checked against the corresponding Ceph source. They are not raw cluster exports. Values and identifiers are
intentionally fictitious; endpoint field names, JSON types, additive release differences, and capability flags model
the checked releases.

Source baselines:

- `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df` — Reef 18.2.8.
- `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a` — Squid 19.2.5.
- `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc` — Tentacle 20.2.2.

The primary source contracts are under `src/pybind/mgr/dashboard/controllers/`, with standalone exporter cadence under
`src/exporter/`. In particular, `controllers/health.py` converts `health.checks` from Ceph's keyed map to an array and
adds each check code as `type`; the release fixtures preserve that Dashboard wire shape. Tests additionally assert the
HTTP paths, API media versions, pagination/query parameters, release capability flags, and selected decoded values used
by the collector.

RGW topology fixtures retain each zonegroup's `is_master` and `master_zone` fields. Dashboard uses `is_master` for the
zonegroup relationship and derives a zone's master state by comparing `master_zone` with the zone ID; the contract tests
assert both rows rather than treating an absent zone-level `is_master` field as false.

The legacy Pacific fixture remains a backward-compatibility parser regression input. It is not an official support
claim.
