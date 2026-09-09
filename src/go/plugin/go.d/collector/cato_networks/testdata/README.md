# Cato Networks Test Fixtures

`centreon-cato-api.mockoon.json` is a compact JSON copy of the mirrored
Centreon plugins repository file:

```text
tests/network/security/cato/networks/api/cato-api.json
```

Source commit: `a4f99c7763517778099681d873609ea7d203a751`

Upstream URL:
https://github.com/centreon/centreon-plugins/blob/a4f99c7763517778099681d873609ea7d203a751/tests/network/security/cato/networks/api/cato-api.json

The source repository is Apache-2.0 licensed. The fixture provides raw Mockoon
GraphQL responses for `entityLookup`, `accountSnapshot`, and `accountMetrics`.

`cato-account-snapshot.schema-shaped.json` is a synthetic account snapshot
response aligned with the current Cato schema. It complements the historical
Centreon fixture, whose `Degraded` connectivity enum is not part of that schema.
