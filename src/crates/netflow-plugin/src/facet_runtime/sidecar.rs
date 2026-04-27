use super::FacetFileContribution;
use crate::facet_catalog::FACET_FIELD_SPECS;
use anyhow::{Context, Result};
use fst::{Automaton, IntoStreamer, Set, SetBuilder, Streamer, automaton::Str};
use memmap2::Mmap;
use std::fs::{self, File};
use std::io::BufWriter;
use std::path::{Path, PathBuf};

pub(crate) fn write_sidecar_files(
    journal_path: &Path,
    contribution: &FacetFileContribution,
) -> Result<()> {
    for spec in FACET_FIELD_SPECS.iter().filter(|spec| spec.uses_sidecar) {
        let values = contribution
            .field(spec.name)
            .map(|store| store.collect_strings(None))
            .unwrap_or_default();
        write_field_sidecar(journal_path, spec.name, &values)?;
    }

    Ok(())
}

pub(crate) fn delete_sidecar_files(journal_path: &Path) {
    for spec in FACET_FIELD_SPECS.iter().filter(|spec| spec.uses_sidecar) {
        let path = sidecar_path(journal_path, spec.name);
        let _ = fs::remove_file(path);
    }
}

pub(crate) fn sidecar_path(journal_path: &Path, field: &str) -> PathBuf {
    PathBuf::from(format!(
        "{}.facet.{}.fst",
        journal_path.display(),
        field.to_ascii_uppercase()
    ))
}

pub(crate) fn search_sidecar(
    journal_path: &Path,
    field: &str,
    term: &str,
    limit: usize,
) -> Result<Vec<String>> {
    let sidecar = sidecar_path(journal_path, field);
    if !sidecar.exists() {
        return Ok(Vec::new());
    }

    let file = File::open(&sidecar)
        .with_context(|| format!("failed to open facet sidecar {}", sidecar.display()))?;
    let mmap = unsafe { Mmap::map(&file) }
        .with_context(|| format!("failed to mmap facet sidecar {}", sidecar.display()))?;
    let set = Set::new(mmap)
        .with_context(|| format!("failed to load facet sidecar {}", sidecar.display()))?;
    let matcher = Str::new(term).starts_with();
    let mut stream = set.search(&matcher).into_stream();
    let mut out = Vec::new();

    while let Some(key) = stream.next() {
        let rendered = String::from_utf8_lossy(key).into_owned();
        out.push(rendered);
        if out.len() >= limit {
            break;
        }
    }

    Ok(out)
}

fn write_field_sidecar(journal_path: &Path, field: &str, values: &[String]) -> Result<()> {
    let sidecar = sidecar_path(journal_path, field);
    if values.is_empty() {
        let _ = fs::remove_file(&sidecar);
        return Ok(());
    }

    if let Some(parent) = sidecar.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("failed to create sidecar directory {}", parent.display()))?;
    }

    let tmp_path = sidecar.with_extension("fst.tmp");
    let writer =
        BufWriter::new(File::create(&tmp_path).with_context(|| {
            format!("failed to create temporary sidecar {}", tmp_path.display())
        })?);
    let mut builder =
        SetBuilder::new(writer).with_context(|| format!("failed to init fst for {}", field))?;
    let mut sorted_values = values.to_vec();
    sorted_values.sort_unstable();
    sorted_values.dedup();

    for value in &sorted_values {
        builder.insert(value).with_context(|| {
            format!("failed to add `{value}` to sidecar {}", tmp_path.display())
        })?;
    }

    builder
        .finish()
        .with_context(|| format!("failed to finalize sidecar {}", tmp_path.display()))?;
    fs::rename(&tmp_path, &sidecar).with_context(|| {
        format!(
            "failed to move temporary sidecar {} to {}",
            tmp_path.display(),
            sidecar.display()
        )
    })?;

    Ok(())
}
