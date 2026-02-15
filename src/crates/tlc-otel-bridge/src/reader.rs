use std::fs::File;
use std::path::{Path, PathBuf};

use parquet::arrow::arrow_reader::{ParquetRecordBatchReader, ParquetRecordBatchReaderBuilder};

/// Find all `.parquet` files in a directory, sorted by name.
pub fn find_parquet_files(dir: &Path) -> anyhow::Result<Vec<PathBuf>> {
    let mut files: Vec<PathBuf> = std::fs::read_dir(dir)?
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|ext| ext == "parquet"))
        .collect();
    files.sort();
    Ok(files)
}

/// Open a parquet file and return a streaming batch reader.
///
/// Batches are read lazily — only one batch is in memory at a time.
pub fn open_parquet_file(path: &Path) -> anyhow::Result<ParquetRecordBatchReader> {
    let file = File::open(path)?;
    let reader = ParquetRecordBatchReaderBuilder::try_new(file)?.build()?;
    Ok(reader)
}
