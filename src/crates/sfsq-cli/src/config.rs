//! Resolve the WAL and SFST directories from `otel.yaml` config files and/or
//! explicit flags.
//!
//! Resolution is per-dir, explicit flag first, then user config, then stock
//! config (mirroring the agent's own stock→user override order). An
//! unresolved dir is a hard error.
//!
//! The agent's `otel.yaml` is typically partial ("Only include the fields you
//! want to change"), so we parse with a minimal struct that extracts only the
//! two dirs and ignores every other field — reusing the full plugin config
//! type would fail on unrelated missing mandatory fields.

use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::Deserialize;

/// Minimal view of `otel.yaml`: only the dirs we need. Unknown fields are
/// ignored; missing sections deserialize to `None` (serde treats `Option`
/// fields as optional).
#[derive(Debug, Default, Deserialize)]
struct OtelYaml {
    logs: Option<LogsSection>,
}

#[derive(Debug, Default, Deserialize)]
struct LogsSection {
    wal: Option<DirOnly>,
    index: Option<DirOnly>,
}

#[derive(Debug, Default, Deserialize)]
struct DirOnly {
    dir: Option<PathBuf>,
}

impl OtelYaml {
    fn wal_dir(&self) -> Option<PathBuf> {
        self.logs.as_ref()?.wal.as_ref()?.dir.clone()
    }
    fn sfst_dir(&self) -> Option<PathBuf> {
        self.logs.as_ref()?.index.as_ref()?.dir.clone()
    }
}

fn load(path: &Path) -> Result<OtelYaml> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("reading config {}", path.display()))?;
    serde_yaml::from_str(&contents).with_context(|| format!("parsing config {}", path.display()))
}

/// The resolved base directories. Per-tenant subdirs (`{dir}/{tenant}`) are
/// applied downstream during discovery.
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

/// Resolve both dirs. Explicit flag wins, then user `--config`, then
/// `--stock-config`. An unresolved dir is an error naming the ways to set it.
pub fn resolve_dirs(inputs: &DirInputs) -> Result<Dirs> {
    let user = inputs.config.map(load).transpose()?;
    let stock = inputs.stock_config.map(load).transpose()?;

    let wal = pick(
        inputs.wal_dir,
        user.as_ref().and_then(OtelYaml::wal_dir),
        stock.as_ref().and_then(OtelYaml::wal_dir),
    )
    .context(
        "WAL dir unresolved: pass --wal-dir, or a --config/--stock-config that sets logs.wal.dir",
    )?;

    let sfst = pick(
        inputs.sfst_dir,
        user.as_ref().and_then(OtelYaml::sfst_dir),
        stock.as_ref().and_then(OtelYaml::sfst_dir),
    )
    .context(
        "SFST dir unresolved: pass --sfst-dir, or a --config/--stock-config that sets logs.index.dir",
    )?;

    Ok(Dirs { wal, sfst })
}

/// Per-dir precedence: explicit flag, then user config value, then stock value.
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
    fn per_dir_precedence_flag_over_user_over_stock() {
        let tmp = tempfile::tempdir().unwrap();
        let stock = write(
            tmp.path(),
            "stock.yaml",
            "logs:\n  wal:\n    dir: /stock/wal\n  index:\n    dir: /stock/idx\n",
        );
        let user = write(
            tmp.path(),
            "user.yaml",
            "logs:\n  wal:\n    dir: /user/wal\n",
        );

        // wal: flag wins; sfst: user has none, falls to stock.
        let dirs = resolve_dirs(&DirInputs {
            stock_config: Some(&stock),
            config: Some(&user),
            wal_dir: Some(Path::new("/flag/wal")),
            sfst_dir: None,
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/flag/wal"));
        assert_eq!(dirs.sfst, PathBuf::from("/stock/idx"));

        // No flag: user wal wins over stock; sfst still from stock.
        let dirs = resolve_dirs(&DirInputs {
            stock_config: Some(&stock),
            config: Some(&user),
            wal_dir: None,
            sfst_dir: None,
        })
        .unwrap();
        assert_eq!(dirs.wal, PathBuf::from("/user/wal"));
        assert_eq!(dirs.sfst, PathBuf::from("/stock/idx"));
    }

    #[test]
    fn unresolved_dir_is_an_error() {
        let tmp = tempfile::tempdir().unwrap();
        // A partial user config with no dirs and no stock/flags → error.
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
