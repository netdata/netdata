# Developer and Contributor Corner

This section is the technical entry point for people who integrate with Netdata, extend the Agent, build packages, report
problems, or contribute changes. It complements the operator documentation: operator guides explain how to deploy and use
Netdata, while these guides explain the interfaces, source components, and contribution workflows behind those features.

## Find the right starting point

- **Diagnose a problem for support or development:** Create a
  [Netdata Support Bundle](/docs/developer-and-contributor-corner/netdata-support-bundle.md). It collects and sanitizes the
  Agent diagnostics needed for a support ticket or bug report.
- **Integrate with Netdata data:** Start with the [REST API](/src/web/api/README.md), then choose a database
  [query method](/src/web/api/queries/README.md), an output [formatter](/src/web/api/formatters/README.md), or
  [Netdata badges](/src/web/api/badges/README.md). These references describe how to select time ranges and dimensions,
  aggregate stored samples, and encode the response for another application.
- **Expose configuration in the UI:** Read the
  [Dynamic Configuration developer guide](/docs/developer-and-contributor-corner/dyncfg.md). It covers configuration
  registration, JSON Schema forms, validation, persistence, and communication between the Agent and a plugin.
- **Write or extend a collector:** Begin with the [external plugins overview](/src/plugins.d/README.md). Go collector
  authors should continue with [How to write a Netdata collector in Go](/src/go/plugin/go.d/docs/how-to-write-a-module.md)
  and follow the repository's metadata, chart, configuration, and testing conventions.
- **Understand metric storage:** Use the [Database Engine reference](/src/database/engine/README.md) for the implementation
  and behavior of Netdata's time-series storage.
- **Build or package the Agent:** The [build overview](build-the-netdata-agent-yourself.md) routes source builders,
  package maintainers, and external build-system integrators to the instructions for their workflow.

## Contribute changes

Before changing code or documentation, read the Netdata organization's
[contribution guidelines](https://github.com/netdata/.github/blob/main/CONTRIBUTING.md). They define the expected workflow
for issues, pull requests, testing, and review. Contributions and community participation are also governed by the
[Code of Conduct](https://github.com/netdata/.github/blob/main/CODE_OF_CONDUCT.md).

Security reports require a different path from ordinary bug reports. Follow the
[Security Policy](https://github.com/netdata/.github/blob/main/SECURITY.md) so potentially sensitive vulnerability details
reach the security team privately instead of being disclosed in a public issue.

## Use implementation documentation carefully

Many pages in this section describe internal APIs and source files. Check the documentation for the Netdata version you
are building or running, because internal interfaces can evolve with the code. When a guide and the source disagree, the
source and its tests describe the behavior of that checkout; please report or fix the stale documentation as part of the
same contribution.
