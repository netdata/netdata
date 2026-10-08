<!-- SPDX-License-Identifier: GPL-3.0-or-later -->

# Ceph Alert Source Snapshots

These files are exact, unmodified snapshots of `monitoring/ceph-mixin/prometheus_alerts.yml` from `ceph/ceph`:

- `reef.yaml`: `efac5a54607c13fa50d4822e50242b86e6e446df` (`v18.2.8`), SHA-256
  `0325e5c481d00c674f7faf759e00f6c5c22028dbcc8bb95491404d600c6f3efd`.
- `squid.yaml`: `abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a` (`v19.2.5`), SHA-256
  `259ee363694d174f46427a443ba0a8952b28df0c17af2bb65a2378511bc321ba`.
- `tentacle.yaml`: `06c2f9c35b67055a8a6fb99d1be236b3c4832ace` (`v20.2.3`), SHA-256
  `09308346d3d143ff128813f1142f1499142799174da4e0505ddad7144b8d8716`.

They provide hermetic source inputs for the adjacent Ceph alert-mapping tests. Do not edit a snapshot in place. Refresh
it from a new approved upstream commit and update every coupled source pin and mapping together.
