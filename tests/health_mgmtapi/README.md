# Health command API tester

The `tests/health_mgmtapi` directory contains an integration test for the
[health management API](/src/web/api/health/README.md). The test exercises authentication, global and selective alert
silencing, health-check disabling, reset behavior, and the `LIST` response against a running local Netdata Agent.

## What the test covers

The script sends commands to `/api/v1/manage/health` and checks their effect through `/api/v1/alarms?all`. Its sequence
verifies:

- a valid management token is accepted and an invalid token is rejected;
- `DISABLE ALL`, `SILENCE ALL`, and `RESET` return the expected responses and state;
- selectors for alert names, charts, contexts, and hosts affect only matching alerts;
- multiple criteria in one selector are combined, and multiple selectors can be added;
- incomplete command and selector combinations return the documented warnings; and
- `LIST` returns the expected JSON after every state transition.

The expected `LIST` responses are stored in `tests/health_mgmtapi/expected_list/`. The test also checks the disabled and
silenced flags of three fixture alerts: `system.cpu.10min_cpu_usage`, `system.cpu.10min_cpu_iowait`, and
`system.load.load_trigger`.

## Requirements

This is not a standalone unit test. It requires:

- a Netdata Agent running at `localhost:19999` with a functional health subsystem;
- the three fixture alerts listed above to be active;
- `curl`, Python 3, and `diff` in the test environment;
- read access to the management API key, normally `@varlibdir_POST@/netdata.api.key`; and
- a configured script generated from `health-cmdapi-test.sh.in` by a Debug CMake build.

CMake substitutes the configured Netdata variable-library directory into the generated script. Running the `.in` template
directly will leave that placeholder unresolved and the API key lookup will fail.

## Run the test

Run the generated script from a working directory that contains the `expected_list` directory. For an in-tree Debug build,
that normally means using the corresponding `tests/health_mgmtapi` directory in the build tree while making the source
fixtures available there.

```bash
./health-cmdapi-test.sh
```

The script prints each command and check, retries asynchronous state checks up to ten times, and exits non-zero when a
response or alert state differs from the fixture. It writes the `LIST` responses into the current directory before comparing
them with the expected JSON files.

## Operational warning

The test changes the running Agent's health-management state. It issues `RESET` between scenarios and again near the end of
the successful sequence, but an interruption or early failure can leave alerts disabled or notifications silenced. Run it
only against a disposable development Agent, not a production monitoring system. After any failed or interrupted run, issue
an authenticated `cmd=RESET` request and verify the current state with `cmd=LIST` before continuing other tests.

