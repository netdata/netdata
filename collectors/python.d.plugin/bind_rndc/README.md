# bind_rndc

Module parses bind dump file to collect real-time performance metrics

**Requirements:**
 * Version of bind must be 9.6 +
 * Netdata must have permissions to run `rndc stats`

It produces:

1. **Name server statistics**
 * requests
 * responses
 * success
 * auth_answer
 * nonauth_answer
 * nxrrset
 * failure
 * nxdomain
 * recursion
 * duplicate
 * rejections

2. **Incoming queries**
 * RESERVED0
 * A
 * NS
 * CNAME
 * SOA
 * PTR
 * MX
 * TXT
 * X25
 * AAAA
 * SRV
 * NAPTR
 * A6
 * DS
 * RSIG
 * DNSKEY
 * SPF
 * ANY
 * DLV

3. **Outgoing queries**
 * Same as Incoming queries


### configuration

Sample:

```yaml
local:
  named_stats_path       : '/var/log/bind/named.stats'
```

If no configuration is given, module will attempt to read named.stats file  at `/var/log/bind/named.stats`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fbind_rndc%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
