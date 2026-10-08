//! Resolve the WAL and SFST directories from `otel.yaml` config files and/or
//! explicit flags.
//!
//! The agent's `otel.yaml` no longer configures per-signal dirs: it sets one
//! `base_dir` and the plugin derives `{base_dir}/{signal}/{wal,index,catalog}`
//! (see `PluginConfig::lifecycle_for`). This module serves only the logs query
//! path, so it derives the logs dirs `{base_dir}/logs/wal` and
//! `{base_dir}/logs/index`, parsing with a minimal struct that extracts just
//! `base_dir` — reusing the full plugin config type would fail on unrelated
//! missing mandatory fields.

use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::Deserialize;

/// Minimal view of `otel.yaml`: unknown fields are ignored, and a missing
/// `base_dir` deserializes to `None`.
#[derive(Debug, Default, Deserialize)]
struct OtelYaml {
    base_dir: Option<PathBuf>,
}

impl OtelYaml {
    /// Logs WAL dir: `{base_dir}/logs/wal`, matching `lifecycle_for(Signal::Logs)`.
    fn wal_dir(&self) -> Option<PathBuf> {
        self.base_dir.as_ref().map(|b| b.join("logs").join("wal"))
    }
    /// Logs SFST/index dir: `{base_dir}/logs/index`, same derivation.
    fn sfst_dir(&self) -> Option<PathBuf> {
        self.base_dir.as_ref().map(|b| b.join("logs").join("index"))
    }
}

fn load(path: &Path) -> Result<OtelYaml> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("reading config {}", path.display()))?;
    serde_yaml::from_str(&contents).with_context(|| format!("parsing config {}", path.display()))
}

/// The resolved WAL and SFST directories, before the per-tenant subdirectory
/// (`{dir}/{tenant}`) is appended during discovery.
#[derive(Debug, Clone)]
pub struct Dirs {
    pub wal: PathBuf,
    pub sfst: PathBuf,
}

/// Inputs to directory resolution, straight from the CLI flags.
#[derive(Debug, Default)]
pub struct DirInputs<'a> {
    pub stock_config: Option<&'a Path>,
    pub config: Option<&'a Path>,
    pub wal_dir: Option<&'a Path>,
    pub sfst_dir: Option<&'a Path>,
}

/// Resolve the WAL and SFST dirs, each independently: explicit flag wins, then
/// user `--config`, then `--stock-config`. An unresolved dir is an error
/// naming the ways to set it.
pub fn resolve_dirs(inputs: &DirInputs) -> Result<Dirs> {
    let user = inputs.config.map(load).transpose()?;
    let stock = inputs.stock_config.map(load).transpose()?;

    let wal = pick(
        inputs.wal_dir,
        user.as_ref().and_then(OtelYaml::wal_dir),
        stock.as_ref().and_then(OtelYaml::wal_dir),
    )
    .context(
        "WAL dir unresolved: pass --wal-dir, or a --config/--stock-config that sets base_dir",
    )?;

    let sfst = pick(
        inputs.sfst_dir,
        user.as_ref().and_then(OtelYaml::sfst_dir),
        stock.as_ref().and_then(OtelYaml::sfst_dir),
    )
    .context(
        "SFST dir unresolved: pass --sfst-dir, or a --config/--stock-config that sets base_dir",
    )?;

    Ok(Dirs { wal, sfst })
}

fn pick(flag: Option<&Path>, user: Option<PathBuf>, stock: Option<PathBuf>) -> Option<PathBuf> {
    flag.map(Path::to_path_buf).or(user).or(stock)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    fn write(dir: &Path, name: &str, body: &str) -> PathBuf {
        let p = dir.join(name);
        let mut f = std::fs::File::create(&p).unwrap();
        f.write_all(body.as_bytes()).unwrap();
        p
    }

    #[test]
    fn dirs_derived_from_base_dir_flag_over_user_over_stock() {
        let tmp = tempfile::tempdir().unwrap();
        let stock = write(tmp.path(), "stock.yaml", "base_dir: /stock/otel\n");
        let user = write(tmp.path(), "user.yaml", "base_dir: /user/otel\n");

        // wal: explicit flag wins; sfst: derived from the user base_dir.
        let dirs = resolve_dirs(&DirInputs {
            stock_config: Some(&stock),
            config: Some(&user),
            wal_dir: Some(Path::new("/flag/wal")),
            sfst_dir: None,
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/flag/wal"));
        assert_eq!(dirs.sfst, PathBuf::from("/user/otel/logs/index"));

        // No flags: both dirs derive from the user base_dir (wins over stock).
        let dirs = resolve_dirs(&DirInputs {
            stock_config: Some(&stock),
            config: Some(&user),
            wal_dir: None,
            sfst_dir: None,
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/user/otel/logs/wal"));
        assert_eq!(dirs.sfst, PathBuf::from("/user/otel/logs/index"));
    }

    #[test]
    fn base_dir_falls_back_to_stock_when_user_omits_it() {
        let tmp = tempfile::tempdir().unwrap();
        let stock = write(tmp.path(), "stock.yaml", "base_dir: /stock/otel\n");
        // A partial user config without base_dir → derive from stock.
        let user = write(
            tmp.path(),
            "user.yaml",
            "endpoint:\n  path: '127.0.0.1:4317'\n",
        );
        let dirs = resolve_dirs(&DirInputs {
            stock_config: Some(&stock),
            config: Some(&user),
            wal_dir: None,
            sfst_dir: None,
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/stock/otel/logs/wal"));
        assert_eq!(dirs.sfst, PathBuf::from("/stock/otel/logs/index"));
    }

    #[test]
    fn unresolved_dir_is_an_error() {
        let tmp = tempfile::tempdir().unwrap();
        // A partial user config with no base_dir and no stock/flags → error.
        let user = write(
            tmp.path(),
            "user.yaml",
            "endpoint:\n  path: '127.0.0.1:4317'\n",
        );
        let err = resolve_dirs(&DirInputs {
            stock_config: None,
            config: Some(&user),
            wal_dir: None,
            sfst_dir: None,
        })
        .unwrap_err();
        assert!(err.to_string().contains("WAL dir unresolved"));
    }

    #[test]
    fn explicit_flags_need_no_config() {
        let dirs = resolve_dirs(&DirInputs {
            stock_config: None,
            config: None,
            wal_dir: Some(Path::new("/w")),
            sfst_dir: Some(Path::new("/s")),
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/w"));
        assert_eq!(dirs.sfst, PathBuf::from("/s"));
    }
}
