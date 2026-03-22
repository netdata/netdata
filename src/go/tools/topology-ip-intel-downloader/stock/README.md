# Stock IP Intelligence Payload

This directory holds the source-controlled stock MMDB payload shipped by
`netdata-plugin-netflow`.

Installed destination:

- `${NETDATA_STOCK_DATA_DIR}/topology-ip-intel`

Current stock source policy:

- provider: `iptoasn`
- source family: combined ASN + country dataset
- license intent: PDDL v1.0, per the product decisions recorded in the local
  topology TODO files

Maintainer refresh command from the repository root:

```bash
./src/go/tools/topology-ip-intel-downloader/refresh-stock.sh
```

Generated files:

- `topology-ip-asn.mmdb`
- `topology-ip-country.mmdb`
- `topology-ip-intel.json`

The JSON metadata file records the exact provider, dataset reference, and
generation timestamp used for the checked-in payload.
