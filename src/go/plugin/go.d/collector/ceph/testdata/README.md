# Ceph Dashboard contract fixtures

These fixtures are synthetic, sanitized contract documents derived from read-only Dashboard validation and then
cross-checked against the corresponding Ceph source. They are not raw cluster exports. Values and identifiers are
intentionally fictitious; endpoint field names, JSON types, additive release differences, and capability flags model
the checked releases.

Source baselines:

- `ceph/ceph @ 618f440892089921c3e944a991122ddc44e60516` — Pacific 16.2.15.
- `ceph/ceph @ b12291d110049b2f35e32e0de30d70e9a4c060d2` — Quincy 17.2.7.
- `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df` — Reef 18.2.8.
- `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a` — Squid 19.2.5.
- `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc` — Tentacle 20.2.2.

The primary source contracts are under `src/pybind/mgr/dashboard/controllers/`, with standalone exporter cadence under
`src/exporter/`. In particular, `controllers/health.py` converts `health.checks` from Ceph's keyed map to an array and
adds each check code as `type`; the release fixtures preserve that Dashboard wire shape. Tests additionally assert the
HTTP paths, API media versions, pagination/query parameters, release capability flags, and selected decoded values used
by the collector.

Pacific and Quincy exercise their source-defined one-shot OSD v1.0 contract; Quincy also exercises the modern FSID
endpoint, while Pacific obtains the FSID from its monitor response. Reef, Squid, and Tentacle exercise OSD v1.1 with a
single whole-list request and the core on-demand Function endpoints. The tests compare the production client's observed
wire requests with an independent literal manifest so changing an implementation constant cannot change the test oracle.
