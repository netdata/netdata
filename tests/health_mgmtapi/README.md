# Health command API tester

The directory `tests/health_cmdapi` contains the test script `health-cmdapi-test.sh` for the [health command API](../../web/api/health).

The script can be executed with options to prepare the system for the tests, run them and restore the system to its previous state. 

It depends on the management API being accessible on localhost:19999 and on the responses to the api/v1/alarms?all requests being functional.
It also requires read access to the management API key that is usually under `/var/lib/netdata/netdata.api.key` (`@varlibdir_POST@/netdata.api.key`).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Ftests%2Fhealth_mgmtapi%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
