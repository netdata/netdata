# energid

A collector for [Energi Core](https://github.com/energicryptocurrency/energi)
node instance monitoring.

As Energi Core Gen 1 & 2 are based on the original Bitcoin code and
supports very similar JSON RPC, there is quite high chance the module works
with many others forks including bitcoind itself.

Introduces several new charts:

1.  **Blockchain Index**
    -   blocks
    -   headers

2.  **Blockchain Difficulty**
    -   diff

3.  **MemPool** in MiB
    -   Max
    -   Usage
    -   TX Size

4.  **Secure Memory** in KiB
    -   Total
    -   Locked
    -   Used

5.  **Network**
    -   Connections

6.  **UTXO** (Unspent Transaction Output)
    -   UTXO
    -   Xfers (related transactions)

Configuration is needed in most cases of secure deployment to specify RPC
credentials. However, Energi, Bitcoin and Dash daemons are checked on
startup by default.

It may be desired to increase retry count for production use due to possibly
long daemon startup.

## Configuration

Sample:
```yaml
energi:
    host: '127.0.0.1'
    port: 9796
    user: energi
    pass: energi

bitcoin:
    host: '127.0.0.1'
    port: 8332
    user: bitcoin
    pass: bitcoin
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fenergid%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
