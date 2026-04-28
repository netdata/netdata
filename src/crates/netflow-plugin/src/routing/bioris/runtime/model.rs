use super::super::{ProtoAfiSafi, ProtoObserveAfiSafi};
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(in crate::routing::bioris) enum AfiSafi {
    Ipv4Unicast,
    Ipv6Unicast,
}

impl AfiSafi {
    pub(in crate::routing::bioris) fn as_proto(self) -> i32 {
        match self {
            Self::Ipv4Unicast => ProtoAfiSafi::IPv4Unicast as i32,
            Self::Ipv6Unicast => ProtoAfiSafi::IPv6Unicast as i32,
        }
    }

    pub(in crate::routing::bioris) fn as_str(self) -> &'static str {
        match self {
            Self::Ipv4Unicast => "ipv4-unicast",
            Self::Ipv6Unicast => "ipv6-unicast",
        }
    }

    pub(in crate::routing::bioris) fn as_observe_proto(self) -> i32 {
        match self {
            Self::Ipv4Unicast => ProtoObserveAfiSafi::IPv4Unicast as i32,
            Self::Ipv6Unicast => ProtoObserveAfiSafi::IPv6Unicast as i32,
        }
    }
}

#[derive(Debug, Clone)]
pub(super) struct ObserveTarget {
    pub(super) router: String,
    pub(super) afisafi: AfiSafi,
}

pub(super) struct ObserveTaskHandle {
    pub(super) cancel: CancellationToken,
    pub(super) handle: JoinHandle<()>,
}
