# NetFlow/IPFIX/sFlow test data attribution

Source: https://github.com/akvorado/akvorado
License: AGPL-3.0 (GPL compatible)

Files (verbatim copies from Akvorado testdata):
- outlet/flow/decoder/sflow/testdata/data-1140.pcap
- outlet/flow/decoder/sflow/testdata/data-discard-interface.pcap
- outlet/flow/decoder/sflow/testdata/data-encap-vxlan.pcap
- outlet/flow/decoder/sflow/testdata/data-icmpv4.pcap
- outlet/flow/decoder/sflow/testdata/data-icmpv6.pcap
- outlet/flow/decoder/netflow/testdata/datalink-data.pcap
- outlet/flow/decoder/netflow/testdata/datalink-template.pcap
- outlet/flow/decoder/sflow/testdata/data-local-interface.pcap
- outlet/flow/decoder/sflow/testdata/data-multiple-interfaces.pcap
- outlet/flow/decoder/netflow/testdata/data.pcap
- outlet/flow/decoder/sflow/testdata/data-qinq.pcap
- outlet/flow/decoder/sflow/testdata/data-sflow-expanded-sample.pcap
- outlet/flow/decoder/sflow/testdata/data-sflow-ipv4-data.pcap
- outlet/flow/decoder/sflow/testdata/data-sflow-raw-ipv4.pcap
- outlet/flow/decoder/netflow/testdata/data+templates.pcap
- outlet/flow/decoder/netflow/testdata/icmp-data.pcap
- outlet/flow/decoder/netflow/testdata/icmp-template.pcap
- outlet/flow/decoder/netflow/testdata/ipfixprobe-data.pcap
- outlet/flow/decoder/netflow/testdata/ipfixprobe-templates.pcap
- outlet/flow/decoder/netflow/testdata/ipfix-srv6-data.pcap
- outlet/flow/decoder/netflow/testdata/ipfix-srv6-template.pcap
- outlet/flow/decoder/netflow/testdata/juniper-cpid-data.pcap
- outlet/flow/decoder/netflow/testdata/juniper-cpid-template.pcap
- outlet/flow/decoder/netflow/testdata/mpls.pcap
- outlet/flow/decoder/netflow/testdata/multiplesamplingrates-data.pcap
- outlet/flow/decoder/netflow/testdata/multiplesamplingrates-options-data.pcap
- outlet/flow/decoder/netflow/testdata/multiplesamplingrates-options-template.pcap
- outlet/flow/decoder/netflow/testdata/multiplesamplingrates-template.pcap
- outlet/flow/decoder/netflow/testdata/nat.pcap
- outlet/flow/decoder/netflow/testdata/nfv5.pcap
- outlet/flow/decoder/netflow/testdata/options-data.pcap
- outlet/flow/decoder/netflow/testdata/options-template.pcap
- outlet/flow/decoder/netflow/testdata/physicalinterfaces.pcap
- outlet/flow/decoder/netflow/testdata/samplingrate-data.pcap
- outlet/flow/decoder/netflow/testdata/samplingrate-template.pcap
- outlet/flow/decoder/netflow/testdata/template.pcap

These files are used by unit/integration-style tests to validate decoding of real NetFlow/IPFIX/sFlow records.

Additional stress-test captures downloaded at test time (license per tcpreplay capture page):
- https://s3.amazonaws.com/tcpreplay-pcap-files/smallFlows.pcap
- https://s3.amazonaws.com/tcpreplay-pcap-files/bigFlows.pcap

Source: https://tcpreplay.appneta.com/wiki/captures.html
