use crate::tiering::TierKind;
use anyhow::{Context, Result};
use bytesize::ByteSize;
use clap::{Parser, ValueEnum};
use rt::NetdataEnv;
use serde::de;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

mod defaults;
mod runtime;
mod types;
mod validation;

use defaults::*;
pub(crate) use types::*;

#[cfg(test)]
#[path = "plugin_config_tests.rs"]
mod tests;
