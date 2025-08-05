//! High-performance chart writer with minimal allocations.

use super::metadata::{ChartMetadata, DimensionMetadata};
use bytes::{BufMut, BytesMut};
use std::io::{self, Write};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

/// High-performance writer for Netdata chart protocol.
///
/// Uses a reusable buffer to minimize allocations. The buffer is reused across
/// multiple chart updates, only growing when necessary.
pub struct ChartWriter {
    buffer: BytesMut,
}

impl ChartWriter {
    /// Create a new chart writer with default capacity (4KB)
    pub fn new() -> Self {
        Self::with_capacity(4096)
    }

    /// Create a new chart writer with specified capacity
    pub fn with_capacity(capacity: usize) -> Self {
        Self {
            buffer: BytesMut::with_capacity(capacity),
        }
    }

    /// Write a chart definition (CHART + DIMENSION commands)
    pub fn write_chart_definition(&mut self, metadata: &ChartMetadata) {
        // CHART command
        self.buffer.put_slice(b"CHART ");
        self.buffer.put_slice(metadata.id.as_bytes());
        self.buffer.put_slice(b" '");
        self.buffer.put_slice(metadata.name.as_bytes());
        self.buffer.put_slice(b"' '");
        self.buffer.put_slice(metadata.title.as_bytes());
        self.buffer.put_slice(b"' '");
        self.buffer.put_slice(metadata.units.as_bytes());
        self.buffer.put_slice(b"' '");
        self.buffer.put_slice(metadata.family.as_bytes());
        self.buffer.put_slice(b"' '");
        self.buffer.put_slice(metadata.context.as_bytes());
        self.buffer.put_slice(b"' ");
        self.buffer.put_slice(metadata.chart_type.as_str().as_bytes());
        self.buffer.put_slice(b" ");
        self.write_i64(metadata.priority);
        self.buffer.put_slice(b" ");
        self.write_u64(metadata.update_every);
        self.buffer.put_u8(b'\n');

        // DIMENSION commands
        for dim in metadata.dimensions.values() {
            self.write_dimension_definition(dim);
        }
    }

    /// Write a dimension definition (DIMENSION command)
    fn write_dimension_definition(&mut self, dim: &DimensionMetadata) {
        self.buffer.put_slice(b"DIMENSION ");
        self.buffer.put_slice(dim.id.as_bytes());
        self.buffer.put_slice(b" '");
        self.buffer.put_slice(dim.name.as_bytes());
        self.buffer.put_slice(b"' ");
        self.buffer.put_slice(dim.algorithm.as_str().as_bytes());
        self.buffer.put_slice(b" ");
        self.write_i64(dim.multiplier);
        self.buffer.put_slice(b" ");
        self.write_i64(dim.divisor);

        if dim.hidden {
            self.buffer.put_slice(b" hidden");
        }

        self.buffer.put_u8(b'\n');
    }

    /// Begin a chart update (BEGIN command)
    ///
    /// The `update_every` specifies the collection interval.
    /// This helps Netdata perform accurate interpolation.
    pub fn begin_chart(&mut self, chart_id: &str, update_every: Duration) {
        self.buffer.put_slice(b"BEGIN ");
        self.buffer.put_slice(chart_id.as_bytes());
        self.buffer.put_u8(b' ');
        // Netdata expects microseconds
        self.write_u64(update_every.as_micros() as u64);
        self.buffer.put_u8(b'\n');
    }

    /// Write a dimension value (SET command)
    pub fn write_dimension(&mut self, dimension_id: &str, value: i64) {
        self.buffer.put_slice(b"SET ");
        self.buffer.put_slice(dimension_id.as_bytes());
        self.buffer.put_slice(b" = ");
        self.write_i64(value);
        self.buffer.put_u8(b'\n');
    }

    /// End a chart update (END command)
    ///
    /// The `collection_time` specifies when the data was collected.
    /// This allows Netdata to accurately align data points and perform proper interpolation.
    pub fn end_chart(&mut self, collection_time: SystemTime) {
        self.buffer.put_slice(b"END ");
        // Netdata expects Unix timestamp in seconds
        let secs = collection_time
            .duration_since(UNIX_EPOCH)
            .unwrap_or(Duration::ZERO)
            .as_secs();
        self.write_u64(secs);
        self.buffer.put_u8(b'\n');
    }

    /// Write an i64 value using itoa (zero-allocation integer formatting)
    #[inline]
    fn write_i64(&mut self, value: i64) {
        let mut buf = itoa::Buffer::new();
        let s = buf.format(value);
        self.buffer.put_slice(s.as_bytes());
    }

    /// Write a u64 value using itoa (zero-allocation integer formatting)
    #[inline]
    fn write_u64(&mut self, value: u64) {
        let mut buf = itoa::Buffer::new();
        let s = buf.format(value);
        self.buffer.put_slice(s.as_bytes());
    }

    /// Flush the buffer to stdout
    pub fn flush(&mut self) -> io::Result<()> {
        let stdout = io::stdout();
        let mut handle = stdout.lock();
        handle.write_all(&self.buffer)?;
        handle.flush()?;
        self.buffer.clear();
        Ok(())
    }

    /// Get the current buffer size (for monitoring/debugging)
    pub fn buffer_len(&self) -> usize {
        self.buffer.len()
    }

    /// Get a reference to the buffer (for testing)
    pub fn buffer(&self) -> &[u8] {
        &self.buffer
    }

    /// Clear the buffer without flushing
    pub fn clear(&mut self) {
        self.buffer.clear();
    }

    /// Append this writer's buffer to another buffer and clear this writer
    pub fn append_to(&mut self, target: &mut BytesMut) {
        target.put(&self.buffer[..]);
        self.buffer.clear();
    }
}

impl Default for ChartWriter {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{ChartMetadata, ChartType, DimensionAlgorithm, DimensionMetadata};

    #[test]
    fn test_write_chart_definition() {
        let mut writer = ChartWriter::new();
        let mut metadata = ChartMetadata::new("test.chart");
        metadata.title = "Test Chart".to_string();
        metadata.units = "widgets".to_string();
        metadata.chart_type = ChartType::Line;

        let mut dim = DimensionMetadata::new("value1");
        dim.algorithm = DimensionAlgorithm::Absolute;
        metadata.dimensions.insert("value1".to_string(), dim);

        writer.write_chart_definition(&metadata);

        let output = String::from_utf8_lossy(&writer.buffer);
        assert!(output.contains("CHART test.chart"));
        assert!(output.contains("Test Chart"));
        assert!(output.contains("DIMENSION value1"));
    }

    #[test]
    fn test_write_chart_update() {
        let mut writer = ChartWriter::new();

        writer.begin_chart("test.chart", Duration::from_secs(1));
        writer.write_dimension("value1", 42);
        writer.write_dimension("value2", 13);
        writer.end_chart(UNIX_EPOCH + Duration::from_secs(1609459200)); // 2021-01-01 00:00:00 UTC

        let output = String::from_utf8_lossy(&writer.buffer);
        assert_eq!(output, "BEGIN test.chart 1000000\nSET value1 = 42\nSET value2 = 13\nEND 1609459200\n");
    }

    #[test]
    fn test_reusable_buffer() {
        let mut writer = ChartWriter::new();

        // First update
        writer.begin_chart("test.chart", Duration::from_secs(1));
        writer.write_dimension("value", 1);
        writer.end_chart(UNIX_EPOCH + Duration::from_secs(1609459200));
        let len1 = writer.buffer_len();
        writer.clear();

        // Second update - buffer should be reused
        writer.begin_chart("test.chart", Duration::from_secs(1));
        writer.write_dimension("value", 2);
        writer.end_chart(UNIX_EPOCH + Duration::from_secs(1609459201));
        let len2 = writer.buffer_len();

        // Buffer capacity should be same (reused)
        assert_eq!(len1, len2);
    }
}
