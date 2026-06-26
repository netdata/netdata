//! Buffer-all fixture builder for reader tests.
//!
//! Deliberately looser than the public [`crate::StreamWriter`]: it can
//! emit files with chunks missing (no SUMR, no META) and accepts its
//! chunks in any call order, so reader tests can pin behavior on
//! partial or odd files. Test scaffolding only — production files are
//! written through [`crate::StreamWriter`], which enforces the full
//! canonical shape.

use std::io::Write;

use chunk_file::container::ContainerBuilder;

use crate::{
    CHUNK_META, CHUNK_PRIMARY, CHUNK_SUMMARY, CHUNK_TIMS, Error, MAGIC, MAX_STREAM_BATCHES,
    VERSION, high_field_id, mid_field_id, stream_batch_id,
};

pub struct FixtureWriter {
    summary: Option<Vec<u8>>,
    metadata: Option<Vec<u8>>,
    primary: Option<Vec<u8>>,
    mid_fields: Vec<Vec<u8>>,
    high_fields: Vec<Vec<u8>>,
    timestamps: Option<Vec<u8>>,
    stream_batches: Vec<Vec<u8>>,
}

impl FixtureWriter {
    pub fn new() -> Self {
        Self {
            summary: None,
            metadata: None,
            primary: None,
            mid_fields: Vec::new(),
            high_fields: Vec::new(),
            timestamps: None,
            stream_batches: Vec::new(),
        }
    }

    pub fn set_summary(&mut self, packed: Vec<u8>) {
        self.summary = Some(packed);
    }

    pub fn set_metadata(&mut self, packed: Vec<u8>) {
        self.metadata = Some(packed);
    }

    pub fn set_primary(&mut self, packed: Vec<u8>) {
        self.primary = Some(packed);
    }

    pub fn add_mid_field(&mut self, packed: Vec<u8>) -> u16 {
        let idx = u16::try_from(self.mid_fields.len()).unwrap();
        self.mid_fields.push(packed);
        idx
    }

    pub fn add_high_field(&mut self, packed: Vec<u8>) -> u16 {
        let idx = u16::try_from(self.high_fields.len()).unwrap();
        self.high_fields.push(packed);
        idx
    }

    pub fn set_timestamps(&mut self, packed: Vec<u8>) {
        self.timestamps = Some(packed);
    }

    pub fn add_stream_batch(&mut self, packed: Vec<u8>) -> u8 {
        assert!(self.stream_batches.len() < MAX_STREAM_BATCHES as usize);
        let idx = self.stream_batches.len() as u8;
        self.stream_batches.push(packed);
        idx
    }

    /// Emit the file in the canonical chunk order, skipping unset
    /// optional chunks. PRIM, TIMS, and at least one stream batch are
    /// asserted — fixtures below that bar build raw containers via
    /// `chunk_file` directly.
    pub fn write_to<W: Write>(&self, w: &mut W) -> Result<(), Error> {
        let primary = self
            .primary
            .as_ref()
            .expect("fixture needs a primary chunk");
        let timestamps = self
            .timestamps
            .as_ref()
            .expect("fixture needs a timestamps chunk");
        assert!(
            !self.stream_batches.is_empty(),
            "fixture needs at least one stream batch"
        );

        let mut container = ContainerBuilder::new(*MAGIC, VERSION);
        if let Some(sum) = &self.summary {
            container.add_chunk(CHUNK_SUMMARY, sum);
        }
        if let Some(meta) = &self.metadata {
            container.add_chunk(CHUNK_META, meta);
        }
        container.add_chunk(CHUNK_TIMS, timestamps);
        container.add_chunk(CHUNK_PRIMARY, primary);
        for (i, chunk) in self.mid_fields.iter().enumerate() {
            container.add_chunk(mid_field_id(i as u16), chunk);
        }
        for (i, chunk) in self.high_fields.iter().enumerate() {
            container.add_chunk(high_field_id(i as u16), chunk);
        }
        for (i, batch) in self.stream_batches.iter().enumerate() {
            container.add_chunk(stream_batch_id(i as u8), batch);
        }
        container.write_to(w)?;
        Ok(())
    }
}
