# nsd

Module uses the `nsd-control stats_noreset` command to provide `nsd` statistics.

**Requirements:**

-   Version of `nsd` must be 4.0+
-   Netdata must have permissions to run `nsd-control stats_noreset`

It produces:

1.  **Queries**

    -   queries

2.  **Zones**

    -   master
    -   slave

3.  **Protocol**

    -   udp
    -   udp6
    -   tcp
    -   tcp6

4.  **Query Type**

    -   A
    -   NS
    -   CNAME
    -   SOA
    -   PTR
    -   HINFO
    -   MX
    -   NAPTR
    -   TXT
    -   AAAA
    -   SRV
    -   ANY

5.  **Transfer**

    -   NOTIFY
    -   AXFR

6.  **Return Code**

    -   NOERROR
    -   FORMERR
    -   SERVFAIL
    -   NXDOMAIN
    -   NOTIMP
    -   REFUSED
    -   YXDOMAIN

Configuration is not needed.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fnsd%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
