use super::*;

pub(crate) fn v9_canonical_key(field: V9Field) -> Option<&'static str> {
    match field {
        V9Field::InBytes => Some("BYTES"),
        V9Field::InPkts => Some("PACKETS"),
        V9Field::Flows => Some("FLOWS"),
        V9Field::IpProtocolVersion => Some("ETYPE"),
        V9Field::Protocol => Some("PROTOCOL"),
        V9Field::SrcTos | V9Field::DstTos => Some("IPTOS"),
        V9Field::TcpFlags => Some("TCP_FLAGS"),
        V9Field::L4SrcPort => Some("SRC_PORT"),
        V9Field::L4DstPort => Some("DST_PORT"),
        V9Field::Ipv4SrcAddr | V9Field::Ipv6SrcAddr => Some("SRC_ADDR"),
        V9Field::Ipv4DstAddr | V9Field::Ipv6DstAddr => Some("DST_ADDR"),
        V9Field::Ipv4NextHop
        | V9Field::BgpIpv4NextHop
        | V9Field::Ipv6NextHop
        | V9Field::BpgIpv6NextHop => Some("NEXT_HOP"),
        V9Field::SrcAs => Some("SRC_AS"),
        V9Field::DstAs => Some("DST_AS"),
        V9Field::InputSnmp => Some("IN_IF"),
        V9Field::OutputSnmp => Some("OUT_IF"),
        V9Field::SrcMask | V9Field::Ipv6SrcMask => Some("SRC_MASK"),
        V9Field::DstMask | V9Field::Ipv6DstMask => Some("DST_MASK"),
        V9Field::Ipv4SrcPrefix => Some("SRC_PREFIX"),
        V9Field::Ipv4DstPrefix => Some("DST_PREFIX"),
        V9Field::SrcVlan => Some("SRC_VLAN"),
        V9Field::DstVlan => Some("DST_VLAN"),
        V9Field::ForwardingStatus => Some("FORWARDING_STATUS"),
        V9Field::SamplingInterval => Some("SAMPLING_RATE"),
        V9Field::Direction => Some("DIRECTION"),
        V9Field::MinTtl | V9Field::MaxTtl => Some("IPTTL"),
        V9Field::Ipv6FlowLabel => Some("IPV6_FLOW_LABEL"),
        V9Field::Ipv4Ident => Some("IP_FRAGMENT_ID"),
        V9Field::FragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        V9Field::ObservationTimeMilliseconds => Some("OBSERVATION_TIME_MILLIS"),
        V9Field::InSrcMac | V9Field::OutSrcMac => Some("SRC_MAC"),
        V9Field::InDstMac | V9Field::OutDstMac => Some("DST_MAC"),
        V9Field::PostNATSourceIPv4Address | V9Field::PostNATSourceIpv6Address => {
            Some("SRC_ADDR_NAT")
        }
        V9Field::PostNATDestinationIPv4Address | V9Field::PostNATDestinationIpv6Address => {
            Some("DST_ADDR_NAT")
        }
        V9Field::PostNATTSourceTransportPort => Some("SRC_PORT_NAT"),
        V9Field::PostNATTDestinationTransportPort => Some("DST_PORT_NAT"),
        _ => None,
    }
}

pub(crate) fn ipfix_canonical_key(field: &IPFixField) -> Option<&'static str> {
    match field {
        IPFixField::IANA(IANAIPFixField::OctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PostOctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::InitiatorOctets) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::PostPacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::InitiatorPackets) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::IpVersion) => Some("ETYPE"),
        IPFixField::IANA(IANAIPFixField::ProtocolIdentifier) => Some("PROTOCOL"),
        IPFixField::IANA(IANAIPFixField::IpClassOfService)
        | IPFixField::IANA(IANAIPFixField::PostIpClassOfService) => Some("IPTOS"),
        IPFixField::IANA(IANAIPFixField::TcpControlBits) => Some("TCP_FLAGS"),
        IPFixField::IANA(IANAIPFixField::SourceTransportPort) => Some("SRC_PORT"),
        IPFixField::IANA(IANAIPFixField::DestinationTransportPort) => Some("DST_PORT"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6address) => Some("SRC_ADDR"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6address) => Some("DST_ADDR"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6prefixLength) => Some("SRC_MASK"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6prefixLength) => Some("DST_MASK"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefix) => Some("SRC_PREFIX"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefix) => Some("DST_PREFIX"),
        IPFixField::IANA(IANAIPFixField::IpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::IpNextHopIpv6address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv6address) => Some("NEXT_HOP"),
        IPFixField::IANA(IANAIPFixField::BgpSourceAsNumber) => Some("SRC_AS"),
        IPFixField::IANA(IANAIPFixField::BgpDestinationAsNumber) => Some("DST_AS"),
        IPFixField::IANA(IANAIPFixField::IngressInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::IngressPhysicalInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::EgressInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::EgressPhysicalInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::VlanId)
        | IPFixField::IANA(IANAIPFixField::Dot1qVlanId) => Some("SRC_VLAN"),
        IPFixField::IANA(IANAIPFixField::PostVlanId)
        | IPFixField::IANA(IANAIPFixField::PostDot1qVlanId) => Some("DST_VLAN"),
        IPFixField::IANA(IANAIPFixField::ForwardingStatus) => Some("FORWARDING_STATUS"),
        IPFixField::IANA(IANAIPFixField::SamplingInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::FlowDirection)
        | IPFixField::IANA(IANAIPFixField::BiflowDirection) => Some("DIRECTION"),
        IPFixField::IANA(IANAIPFixField::MinimumTtl) | IPFixField::IANA(IANAIPFixField::IpTtl) => {
            Some("IPTTL")
        }
        IPFixField::IANA(IANAIPFixField::FlowLabelIpv6) => Some("IPV6_FLOW_LABEL"),
        IPFixField::IANA(IANAIPFixField::FragmentIdentification) => Some("IP_FRAGMENT_ID"),
        IPFixField::IANA(IANAIPFixField::FragmentOffset) => Some("IP_FRAGMENT_OFFSET"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv4) => Some("ICMPV4_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv4) => Some("ICMPV4_CODE"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv6) => Some("ICMPV6_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv6) => Some("ICMPV6_CODE"),
        IPFixField::IANA(IANAIPFixField::SourceMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostSourceMacaddress) => Some("SRC_MAC"),
        IPFixField::IANA(IANAIPFixField::DestinationMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostDestinationMacaddress) => Some("DST_MAC"),
        IPFixField::IANA(IANAIPFixField::PostNatsourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatsourceIpv6address) => Some("SRC_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv6address) => Some("DST_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptsourceTransportPort) => Some("SRC_PORT_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptdestinationTransportPort) => Some("DST_PORT_NAT"),
        _ => None,
    }
}

pub(crate) fn reverse_ipfix_canonical_key(
    field: &ReverseInformationElement,
) -> Option<&'static str> {
    match field {
        ReverseInformationElement::ReverseOctetDeltaCount
        | ReverseInformationElement::ReversePostOctetDeltaCount
        | ReverseInformationElement::ReverseInitiatorOctets
        | ReverseInformationElement::ReverseResponderOctets => Some("BYTES"),
        ReverseInformationElement::ReversePacketDeltaCount
        | ReverseInformationElement::ReversePostPacketDeltaCount
        | ReverseInformationElement::ReverseInitiatorPackets
        | ReverseInformationElement::ReverseResponderPackets => Some("PACKETS"),
        ReverseInformationElement::ReverseProtocolIdentifier => Some("PROTOCOL"),
        ReverseInformationElement::ReverseIpClassOfService
        | ReverseInformationElement::ReversePostIpClassOfService => Some("IPTOS"),
        ReverseInformationElement::ReverseTcpControlBits => Some("TCP_FLAGS"),
        ReverseInformationElement::ReverseSourceTransportPort
        | ReverseInformationElement::ReverseUdpSourcePort
        | ReverseInformationElement::ReverseTcpSourcePort => Some("SRC_PORT"),
        ReverseInformationElement::ReverseDestinationTransportPort
        | ReverseInformationElement::ReverseUdpDestinationPort
        | ReverseInformationElement::ReverseTcpDestinationPort => Some("DST_PORT"),
        ReverseInformationElement::ReverseSourceIPv4Address
        | ReverseInformationElement::ReverseSourceIPv6Address => Some("SRC_ADDR"),
        ReverseInformationElement::ReverseDestinationIPv4Address
        | ReverseInformationElement::ReverseDestinationIPv6Address => Some("DST_ADDR"),
        ReverseInformationElement::ReverseSourceIPv4PrefixLength
        | ReverseInformationElement::ReverseSourceIPv6PrefixLength => Some("SRC_MASK"),
        ReverseInformationElement::ReverseDestinationIPv4PrefixLength
        | ReverseInformationElement::ReverseDestinationIPv6PrefixLength => Some("DST_MASK"),
        ReverseInformationElement::ReverseIpNextHopIPv4Address
        | ReverseInformationElement::ReverseIpNextHopIPv6Address
        | ReverseInformationElement::ReverseBgpNextHopIPv4Address
        | ReverseInformationElement::ReverseBgpNextHopIPv6Address => Some("NEXT_HOP"),
        ReverseInformationElement::ReverseBgpSourceAsNumber => Some("SRC_AS"),
        ReverseInformationElement::ReverseBgpDestinationAsNumber => Some("DST_AS"),
        ReverseInformationElement::ReverseIngressInterface => Some("IN_IF"),
        ReverseInformationElement::ReverseEgressInterface => Some("OUT_IF"),
        ReverseInformationElement::ReverseVlanId => Some("SRC_VLAN"),
        ReverseInformationElement::ReversePostVlanId => Some("DST_VLAN"),
        ReverseInformationElement::ReverseSourceMacAddress
        | ReverseInformationElement::ReversePostSourceMacAddress => Some("SRC_MAC"),
        ReverseInformationElement::ReverseDestinationMacAddress
        | ReverseInformationElement::ReversePostDestinationMacAddress => Some("DST_MAC"),
        ReverseInformationElement::ReverseForwardingStatus => Some("FORWARDING_STATUS"),
        ReverseInformationElement::ReverseSamplingInterval
        | ReverseInformationElement::ReverseSamplerRandomInterval => Some("SAMPLING_RATE"),
        ReverseInformationElement::ReverseFlowDirection => Some("DIRECTION"),
        ReverseInformationElement::ReverseMinimumTTL
        | ReverseInformationElement::ReverseMaximumTTL => Some("IPTTL"),
        ReverseInformationElement::ReverseFlowLabelIPv6 => Some("IPV6_FLOW_LABEL"),
        ReverseInformationElement::ReverseFragmentIdentification => Some("IP_FRAGMENT_ID"),
        ReverseInformationElement::ReverseFragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        ReverseInformationElement::ReverseIcmpTypeIPv4 => Some("ICMPV4_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv4 => Some("ICMPV4_CODE"),
        ReverseInformationElement::ReverseIcmpTypeIPv6 => Some("ICMPV6_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv6 => Some("ICMPV6_CODE"),
        ReverseInformationElement::ReverseIpVersion => Some("ETYPE"),
        _ => None,
    }
}
