# Collector Authoring Checklist

This file is a quick landing page for humans and AI assistants contributing to the IBM.d plugin. For the full documentation follow these links:

- [Plugin overview & build instructions](README.md)
- [IBM.D framework internals](framework/README.md)
- [Go collector best practices](BEST-PRACTICES.md)

When implementing or modifying a collector:

1. Read the framework guide to understand contexts, generators, and the module layout.
2. Keep `contexts.yaml`, `config.go`, metadata, schema, README, and health alerts synchronized by running `go generate`.
3. Use conservative defaults – configuration always overrides auto-detection.
4. Log clearly, avoid spamming, and never fabricate metric values.
5. Verify with `ibm.d.plugin --dump` before sending a pull request.

That’s it! Dive into the module directories for concrete examples.
