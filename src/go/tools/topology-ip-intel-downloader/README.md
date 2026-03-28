<!-- markdownlint-disable-file MD013 MD043 -->

# topology-ip-intel-downloader

Downloads and converts IP intelligence datasets into MaxMind DB files used by Netdata topology and flow features.

Outputs:

- ASN database (`autonomous_system_number`, `autonomous_system_organization`)
- Country database (`country.iso_code`)
- Metadata JSON with generation details and source provenance

Both MMDB outputs also include Netdata classification metadata under `netdata.*` for CIDRs that must be tracked individually.

## Config lookup

If `--config` is not provided, the tool loads the first existing file from:

1. `/etc/netdata/topology-ip-intel.yaml`
2. `/usr/lib/netdata/conf.d/topology-ip-intel.yaml`

These defaults are build-time Netdata install paths. On prefixed installs (for example `/opt/netdata`), the compiled binary uses the corresponding prefixed directories.

## Run

```bash
cd src/go
go run ./tools/topology-ip-intel-downloader --config /etc/netdata/topology-ip-intel.yaml
```

When installed with Netdata, run it directly as:

```bash
topology-ip-intel-downloader --config /etc/netdata/topology-ip-intel.yaml
```

## Refresh stock payload for packaging

Netdata ships a stock MMDB payload under:

- `${NETDATA_STOCK_DATA_DIR}/topology-ip-intel/topology-ip-asn.mmdb`
- `${NETDATA_STOCK_DATA_DIR}/topology-ip-intel/topology-ip-country.mmdb`

To refresh the source-controlled stock payload used by packages and source installs:

```bash
./src/go/tools/topology-ip-intel-downloader/refresh-stock.sh
```

This regenerates:

- `src/go/tools/topology-ip-intel-downloader/stock/topology-ip-intel/topology-ip-asn.mmdb`
- `src/go/tools/topology-ip-intel-downloader/stock/topology-ip-intel/topology-ip-country.mmdb`
- `src/go/tools/topology-ip-intel-downloader/stock/topology-ip-intel/topology-ip-intel.json`

## Supported sources

- `provider: iptoasn` (default; PDDL)
- `provider: dbip_lite`
- `provider: custom` (mix-and-match datasets)

Supported dataset formats:

- `iptoasn_combined_tsv`
- `dbip_asn_csv`
- `dbip_country_csv`

## Atomic updates

The downloader writes temporary files in the destination directory and atomically renames them over existing files.

This allows live readers to keep using old descriptors while new readers open the new file paths.
