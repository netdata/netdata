use super::FacetFileContribution;
use crate::facet_catalog::{AutocompleteMatchKind, FACET_FIELD_SPECS};
use anyhow::{Context, Result};
use fst::{Automaton, IntoStreamer, Set, SetBuilder, Streamer, automaton::Str};
use memchr::memmem::Finder;
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
        let _ = fs::remove_file(&path);
        let _ = fs::remove_file(path.with_extension("fst.tmp"));
    }
}

pub(crate) fn sidecar_path(journal_path: &Path, field: &str) -> PathBuf {
    let mut path = journal_path.as_os_str().to_os_string();
    path.push(format!(".facet.{}.fst", field.to_ascii_uppercase()));
    PathBuf::from(path)
}

pub(super) fn journal_sidecar_bytes(journal_path: &Path) -> journal_sdk_log_writer::Result<u64> {
    let mut total = 0_u64;

    for spec in FACET_FIELD_SPECS.iter().filter(|spec| spec.uses_sidecar) {
        let path = sidecar_path(journal_path, spec.name);
        match fs::symlink_metadata(&path) {
            Ok(metadata) if metadata.file_type().is_file() => {
                total = total.saturating_add(metadata.len());
            }
            Ok(_) => {
                return Err(journal_sdk_log_writer::WriterError::InvalidPath(format!(
                    "facet sidecar is not a regular file: {}",
                    path.display()
                )));
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(error) => return Err(error.into()),
        }
    }

    Ok(total)
}

pub(crate) fn search_sidecar(
    journal_path: &Path,
    field: &str,
    term: &str,
    limit: usize,
    match_kind: AutocompleteMatchKind,
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

    let mut out = Vec::new();
    match match_kind {
        AutocompleteMatchKind::Prefix => {
            // FST automaton can prune the trie; cheap.
            let matcher = Str::new(term).starts_with();
            let mut stream = set.search(&matcher).into_stream();
            while let Some(key) = stream.next() {
                out.push(String::from_utf8_lossy(key).into_owned());
                if out.len() >= limit {
                    break;
                }
            }
        }
        AutocompleteMatchKind::Substring => {
            // FST has no substring automaton; stream every key and use a
            // SIMD-accelerated finder. Bounded by `limit` early-stop.
            let finder = Finder::new(term.as_bytes());
            let mut stream = set.stream();
            while let Some(key) = stream.next() {
                if finder.find(key).is_some() {
                    out.push(String::from_utf8_lossy(key).into_owned());
                    if out.len() >= limit {
                        break;
                    }
                }
            }
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
    // The workspace `fst` fork allocates builder scratch in a bumpalo arena
    // that must outlive the builder.
    let bump = bumpalo::Bump::new();
    let mut builder = SetBuilder::new(writer, &bump)
        .with_context(|| format!("failed to init fst for {}", field))?;
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

#[cfg(test)]
mod tests {
    use super::*;

    fn sidecar_fields() -> Vec<&'static str> {
        FACET_FIELD_SPECS
            .iter()
            .filter(|spec| spec.uses_sidecar)
            .map(|spec| spec.name)
            .collect()
    }

    #[test]
    fn journal_sidecar_bytes_counts_only_catalog_owned_final_files() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let journal_path = tmp.path().join("flow.journal");
        let fields = sidecar_fields();
        assert!(
            fields.len() >= 2,
            "test requires at least two sidecar fields"
        );

        fs::write(sidecar_path(&journal_path, fields[0]), vec![0_u8; 13])
            .expect("write first owned sidecar");
        fs::write(sidecar_path(&journal_path, fields[1]), vec![0_u8; 29])
            .expect("write second owned sidecar");
        fs::write(
            sidecar_path(&journal_path, fields[0]).with_extension("fst.tmp"),
            vec![0_u8; 101],
        )
        .expect("write temporary sidecar");
        fs::write(
            sidecar_path(&journal_path, "UNKNOWN_FIELD"),
            vec![0_u8; 103],
        )
        .expect("write unknown sidecar");
        fs::write(tmp.path().join("facet-state.bin"), vec![0_u8; 107])
            .expect("write shared facet state");
        fs::write(
            sidecar_path(&tmp.path().join("flow.journal.backup"), fields[0]),
            vec![0_u8; 109],
        )
        .expect("write prefix-collision sidecar");

        assert_eq!(
            journal_sidecar_bytes(&journal_path).expect("measure owned sidecars"),
            42
        );
    }

    #[test]
    fn journal_sidecar_bytes_handles_missing_and_rejects_non_regular_paths() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let journal_path = tmp.path().join("flow.journal");
        let field = sidecar_fields().into_iter().next().expect("sidecar field");
        let path = sidecar_path(&journal_path, field);

        assert_eq!(
            journal_sidecar_bytes(&journal_path).expect("measure missing sidecars"),
            0
        );

        fs::create_dir(&path).expect("create directory at exact sidecar path");
        let error = journal_sidecar_bytes(&journal_path)
            .expect_err("directory at exact sidecar path must fail");
        assert!(matches!(
            error,
            journal_sdk_log_writer::WriterError::InvalidPath(_)
        ));
        fs::remove_dir(&path).expect("remove exact sidecar directory");

        #[cfg(unix)]
        {
            use std::os::unix::fs::symlink;

            let target = tmp.path().join("target");
            fs::write(&target, b"target").expect("write symlink target");
            symlink(&target, &path).expect("create sidecar symlink");
            let error = journal_sidecar_bytes(&journal_path)
                .expect_err("symlink at exact sidecar path must fail");
            assert!(matches!(
                error,
                journal_sdk_log_writer::WriterError::InvalidPath(_)
            ));
        }
    }

    #[test]
    fn delete_sidecar_files_removes_owned_final_and_temporary_files() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let journal_path = tmp.path().join("flow.journal");
        let field = sidecar_fields().into_iter().next().expect("sidecar field");
        let final_path = sidecar_path(&journal_path, field);
        let temporary_path = final_path.with_extension("fst.tmp");
        let unknown_path = sidecar_path(&journal_path, "UNKNOWN_FIELD");

        fs::write(&final_path, b"final").expect("write final sidecar");
        fs::write(&temporary_path, b"temporary").expect("write temporary sidecar");
        fs::write(&unknown_path, b"unknown").expect("write unknown sidecar");

        delete_sidecar_files(&journal_path);

        assert!(!final_path.exists());
        assert!(!temporary_path.exists());
        assert!(unknown_path.exists());
    }

    #[cfg(unix)]
    #[test]
    fn sidecar_path_preserves_non_utf8_journal_paths() {
        use std::ffi::OsString;
        use std::os::unix::ffi::{OsStrExt, OsStringExt};

        let mut expected = b"flow-\xff.journal".to_vec();
        let journal_path = PathBuf::from(OsString::from_vec(expected.clone()));
        expected.extend_from_slice(b".facet.SRC_ADDR.fst");

        assert_eq!(
            sidecar_path(&journal_path, "src_addr")
                .as_os_str()
                .as_bytes(),
            expected
        );
    }
}
