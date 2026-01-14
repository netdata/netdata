//! UI response types for rendering data in the Netdata dashboard.
//!
//! This module provides types for converting histogram responses into UI-friendly formats,
//! including facets, charts, and data points formatted for the Netdata dashboard.

use serde::{Deserialize, Serialize};

// ============================================================================
// UI Response Types (flat structure)
// ============================================================================

/// Top-level response containing facets, available histograms, and a histogram.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Response {
    pub facets: Vec<Facet>,
    pub available_histograms: Vec<AvailableHistogram>,
    pub histogram: Histogram,
}

/// Represents an available histogram option.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct AvailableHistogram {
    pub id: String,
    pub name: String,
    pub order: usize,
}

/// A facet represents a field with multiple value options.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Facet {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub options: Vec<FacetOption>,
}

/// A single option within a facet.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FacetOption {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub count: usize,
}

/// A histogram for a specific field.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Histogram {
    pub id: String,
    pub name: String,
    pub chart: Chart,
}

impl Histogram {
    pub fn count(&self) -> usize {
        self.chart.count()
    }
}

/// A chart containing view metadata and result data.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Chart {
    pub view: ChartView,
    pub result: ChartResult,
}

impl Chart {
    pub fn count(&self) -> usize {
        self.result.count()
    }
}

/// Chart view metadata describing how to display the chart.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartView {
    pub title: String,
    pub after: u32,
    pub before: u32,
    pub update_every: u32,
    pub units: String,
    pub chart_type: String,
    pub dimensions: ChartDimensions,
}

/// Dimensions for the chart view.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartDimensions {
    pub ids: Vec<String>,
    pub names: Vec<String>,
    pub units: Vec<String>,
}

/// Chart result data containing labels, point metadata, and time series data.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartResult {
    pub labels: Vec<String>,
    pub point: ChartPoint,
    pub data: Vec<DataPoint>,
}

impl ChartResult {
    pub fn count(&self) -> usize {
        let mut n = 0;

        for dp in self.data.iter() {
            for item in dp.items.iter() {
                n += item[0];
            }
        }

        n
    }
}

/// Point metadata for chart result.
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartPoint {
    pub value: u64,
    pub arp: u64,
    pub pa: u64,
}

/// A single data point in a time series.
#[derive(Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct DataPoint {
    pub timestamp: u64,
    pub items: Vec<[usize; 3]>,
}

/// Custom serialization for DataPoint to flatten the structure.
///
/// Serialize as: [timestamp, [val1, arp1, pa1], [val2, arp2, pa2], ...]
/// This format matches the expected Netdata chart data format where the first
/// element is the timestamp followed by dimension data arrays.
impl Serialize for DataPoint {
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeSeq;

        // Create a sequence with length = 1 (timestamp) + number of items
        let mut seq = serializer.serialize_seq(Some(1 + self.items.len()))?;

        // First element: timestamp
        seq.serialize_element(&self.timestamp)?;

        // Remaining elements: each [usize; 3] array
        for item in &self.items {
            seq.serialize_element(item)?;
        }

        seq.end()
    }
}

impl<'de> Deserialize<'de> for DataPoint {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        use serde::de::{SeqAccess, Visitor};

        struct DataPointVisitor;

        impl<'de> Visitor<'de> for DataPointVisitor {
            type Value = DataPoint;

            fn expecting(&self, formatter: &mut std::fmt::Formatter) -> std::fmt::Result {
                formatter.write_str("an array with timestamp followed by data items")
            }

            fn visit_seq<A>(self, mut seq: A) -> std::result::Result<Self::Value, A::Error>
            where
                A: SeqAccess<'de>,
            {
                // First element: timestamp
                let timestamp = seq
                    .next_element()?
                    .ok_or_else(|| serde::de::Error::invalid_length(0, &self))?;

                // Remaining elements: collect all [usize; 3] arrays
                let mut items = Vec::new();
                while let Some(item) = seq.next_element()? {
                    items.push(item);
                }

                Ok(DataPoint { timestamp, items })
            }
        }

        deserializer.deserialize_seq(DataPointVisitor)
    }
}
