//! Single source of truth for the OTel signal axis.
//!
//! A signal's identity is two values that must always agree: a numeric
//! `pipeline_id` (stamped into every `FileId`, serialized into filenames and WAL
//! frames, used by the ledger to route events to the owning pipeline, and keying
//! each signal's own `frame_seq` stream) and a remote-key segment
//! (`v2/{segment}/...`). [`Signal`] is the one place that maps
//! a signal to that pair, so the two cannot drift apart; downstream layers carry
//! `Signal` (or its opaque [`SignalSpec`]) instead of loose `(u16, &str)` pairs.
//!
//! The numeric axis is the wire/disk format: it is encoded as `u16` and MUST stay
//! byte-compatible. `Signal` itself is never serialized — filenames, WAL frames,
//! and substrate IPC carry the raw `u16`, and signal-aware code re-derives the
//! enum with [`Signal::try_from`] at that boundary (the handlers live in
//! `otel-ledger`). An id with no known signal is a runtime error (it can come
//! from a persisted file or another process), never a panic.

use std::fmt;

/// An OTel signal handled by the storage substrate: the single mapping from a
/// signal to its `pipeline_id` and remote-key segment (contract in the module
/// docs). The axis is a closed set, so consumers match over it exhaustively;
/// out-of-set ids fail at the [`Signal::try_from`] boundary.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Signal {
    Logs,
    Traces,
}

impl Signal {
    /// The opaque numeric axis stamped into this signal's `FileId`s and used as
    /// the ledger's routing key. Stable and serialized into filenames / WAL
    /// frames — do not renumber.
    pub const fn pipeline_id(self) -> u16 {
        match self {
            Signal::Logs => 0,
            Signal::Traces => 1,
        }
    }

    /// The remote-key segment for this signal (`v2/{segment}/...`), also the
    /// name of its local `{base_dir}/{segment}/` subtree (derived in
    /// [`PluginConfig::lifecycle_for`](crate::config::PluginConfig)).
    pub const fn segment(self) -> &'static str {
        match self {
            Signal::Logs => "logs",
            Signal::Traces => "traces",
        }
    }

    /// The opaque [`SignalSpec`] handed down to the content-agnostic substrate.
    pub const fn spec(self) -> SignalSpec {
        SignalSpec {
            pipeline_id: self.pipeline_id(),
            segment: self.segment(),
        }
    }
}

/// Error decoding a `pipeline_id` (read from a filename, a WAL frame, or an
/// agnostic IPC echo) that corresponds to no known [`Signal`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct UnknownPipelineId(pub u16);

impl fmt::Display for UnknownPipelineId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "unknown pipeline_id {}", self.0)
    }
}

impl std::error::Error for UnknownPipelineId {}

impl TryFrom<u16> for Signal {
    type Error = UnknownPipelineId;

    fn try_from(pipeline_id: u16) -> Result<Self, Self::Error> {
        match pipeline_id {
            0 => Ok(Signal::Logs),
            1 => Ok(Signal::Traces),
            other => Err(UnknownPipelineId(other)),
        }
    }
}

/// The opaque identity of a signal as seen by the content-agnostic substrate: the
/// numeric routing axis plus the remote-key segment, bundled so they cannot be
/// mismatched. Constructible only via [`Signal::spec`]; the substrate
/// (`file-lifecycle`'s `Pipeline`) stores and echoes it without knowing the
/// signal set (it never names "logs"/"traces").
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SignalSpec {
    pipeline_id: u16,
    segment: &'static str,
}

impl SignalSpec {
    /// The numeric routing axis (stamped into this signal's `FileId`s).
    pub const fn pipeline_id(&self) -> u16 {
        self.pipeline_id
    }

    /// The remote-key segment (`v2/{segment}/...`).
    pub const fn segment(&self) -> &'static str {
        self.segment
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The boundary decode is the single line of defense against a bad numeric
    /// id re-entering signal-aware code (WAL events, cleaner/uploader echoes), so
    /// pin that every signal's id round-trips back to itself — a swapped or stale
    /// `try_from` arm would silently mis-route production telemetry otherwise.
    #[test]
    fn pipeline_id_round_trips_through_try_from() {
        for signal in [Signal::Logs, Signal::Traces] {
            assert_eq!(Signal::try_from(signal.pipeline_id()), Ok(signal));
        }
    }

    /// The numeric ids are serialized into on-disk filenames (`-{pipeline_id:05}-`)
    /// and WAL frames, so they are a wire contract: pin the exact values so a
    /// renumber can't silently change what existing files decode to.
    #[test]
    fn pipeline_ids_are_the_stable_wire_values() {
        assert_eq!(Signal::Logs.pipeline_id(), 0);
        assert_eq!(Signal::Traces.pipeline_id(), 1);
    }

    #[test]
    fn unknown_pipeline_id_is_rejected() {
        // 0 and 1 are the only known ids today; everything else must be an error
        // (a logged drop at the boundary, never a panic or a mis-route).
        assert_eq!(Signal::try_from(2), Err(UnknownPipelineId(2)));
        assert_eq!(Signal::try_from(u16::MAX), Err(UnknownPipelineId(u16::MAX)));
        assert_eq!(UnknownPipelineId(2).to_string(), "unknown pipeline_id 2");
    }

    /// `SignalSpec` is the opaque pair handed to the substrate; it must carry the
    /// same axis/segment the `Signal` exposes directly.
    #[test]
    fn spec_matches_signal() {
        for signal in [Signal::Logs, Signal::Traces] {
            let spec = signal.spec();
            assert_eq!(spec.pipeline_id(), signal.pipeline_id());
            assert_eq!(spec.segment(), signal.segment());
        }
    }
}
