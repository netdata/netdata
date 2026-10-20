<!-- markdownlint-disable MD043 -->

# SNMP Trap Pcap Fixtures

These fixtures are generated classic pcap frames, not customer or production captures.
They use documentation IP ranges and the public SNMP community string `public`.

The vendor-oriented fixtures exercise enterprise OID/PEN shapes only. They are not
claims about real device event semantics.

PEN references checked while creating the fixtures:

- Cisco: IANA PEN 9, `1.3.6.1.4.1.9`
- Hewlett-Packard: IANA PEN 11, `1.3.6.1.4.1.11`
- Juniper Networks: IANA PEN 2636, `1.3.6.1.4.1.2636`
- Arista Networks: IANA PEN 30065, `1.3.6.1.4.1.30065`
- Aruba Networks: public OID reference `1.3.6.1.4.1.14823`

The `.pcap.hex` files are hex-encoded so they stay readable in review. Convert
one to a binary pcap with:

```sh
xxd -r -p v2c_coldstart.pcap.hex v2c_coldstart.pcap
```
