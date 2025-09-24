# IBM.d Developer Landing Page

Use this as a jumping-off point when working inside the IBM.d plugin:

- [Project overview](./README.md)
- [IBM.D framework guide](framework/README.md)
- [General collector best practices](../BEST-PRACTICES.md)

Module workflow:

1. Update `contexts/contexts.yaml` and `config.go`.
2. Run `go generate` in the module directory (invokes docgen/metricgen).
3. Sync metadata, config schema, README, and health alerts.
4. Validate with `script -c 'sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m MODULE --dump=3s --dump-summary 2>&1' /dev/null`.

Each module directory (`modules/<name>/`) contains its own README with module-specific notes.
