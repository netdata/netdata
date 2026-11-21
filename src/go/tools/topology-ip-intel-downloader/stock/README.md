<!-- markdownlint-disable-file MD043 -->

# Stock IP Intelligence Payload

This directory documents the stock MMDB payload shipped by native
`netdata-ip-intel-data` packages, official static installers and DEM Docker images.
The native NetFlow and DEM plugins both depend on the data package. DEM/RUM uses
the GEO database without requiring the NetFlow listener.

The generated stock databases are not stored in Git. Package/release builds
stage them into the build workspace from CI/release tooling.

Installed destination:

- `${NETDATA_STOCK_DATA_DIR}/topology-ip-intel`

Current stock source policy:

- ASN source: `dbip:asn-lite@mmdb`
- GEO source: `dbip:city-lite@mmdb`
- output model:
  - `topology-ip-asn.mmdb`
  - `topology-ip-geo.mmdb`
  - `topology-ip-intel.json`

Redistribution note:

- DB-IP Lite downloads are distributed by DB-IP under CC BY 4.0
- Netdata must keep the matching attribution in `REDISTRIBUTED.md`

Maintainer helper command from the repository root:

```bash
./src/go/tools/topology-ip-intel-downloader/refresh-stock.sh
```

This creates a local staging directory at:

- `./artifacts/topology-ip-intel-stock`

The staged JSON metadata file records the exact source list, resolved download
references, range counts, and generation timestamp used for that payload.
