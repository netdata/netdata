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
