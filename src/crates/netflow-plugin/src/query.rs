use crate::flow::canonical_flow_field_names;
use crate::plugin_config::PluginConfig;
use crate::presentation;
use crate::tiering::TierKind;
#[cfg(test)]
use crate::tiering::{OpenTierRow, TierFlowIndexStore};
use anyhow::{Context, Result};
use chrono::Utc;
use hashbrown::HashMap as FastHashMap;
use journal_common::{Seconds, load_machine_id};
use journal_core::file::{JournalFileMap, Mmap};
use journal_core::{
    Direction as JournalDirection, JournalCursor, JournalFile, JournalReader, Location,
};
use journal_registry::{FileInfo, Monitor, Registry, repository::File as RegistryFile};
use crate::flow_index::{
    FieldKind as IndexFieldKind, FieldSpec as IndexFieldSpec, FieldValue as IndexFieldValue,
    FlowId as IndexedFlowId, FlowIndex,
};
use notify::Event;
use regex::Regex;
use serde::de::Error as _;
use serde::{Deserialize, Deserializer};
use serde_json::{Map, Value, json};
use std::borrow::Cow;
use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock};
use std::time::Instant;
use tokio::sync::mpsc::UnboundedReceiver;

mod execution;
mod facets;
mod fields;
mod flows;
mod grouping;
mod metrics;
mod planner;
mod projected;
mod request;
mod scan;
mod service;
mod timeseries;

pub(crate) use execution::*;
pub(crate) use facets::*;
pub(crate) use fields::*;
#[allow(unused_imports)]
pub(crate) use flows::*;
pub(crate) use grouping::*;
#[allow(unused_imports)]
pub(crate) use metrics::*;
#[allow(unused_imports)]
pub(crate) use planner::*;
pub(crate) use projected::*;
pub(crate) use request::*;
pub(crate) use scan::*;
pub(crate) use service::*;
pub(crate) use timeseries::*;

#[cfg(test)]
mod tests;
