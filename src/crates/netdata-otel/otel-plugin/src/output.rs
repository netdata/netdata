//! Output formatting for Netdata plugin protocol emission.

use std::fmt::{self, Write as _};

/// The Netdata chart type.
#[derive(Debug, Clone, Copy, Default)]
pub enum ChartType {
    #[default]
    Line,
    Heatmap,
}

impl fmt::Display for ChartType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ChartType::Line => f.write_str("line"),
            ChartType::Heatmap => f.write_str("heatmap"),
        }
    }
}

/// Fixed precision divisor for float-to-integer scaling.
///
/// Netdata SET values are integers. To preserve decimal precision,
/// we multiply the float value by this divisor before truncating to i64,
/// and declare the same divisor in the DIMENSION line so Netdata divides
/// it back out for display: `displayed = SET_value * 1 / DIVISOR`.
pub const PRECISION_DIVISOR: i64 = 1000;

/// Wrapper that writes a string with single quotes replaced by double quotes.
///
/// The Netdata plugin protocol uses single quotes to delimit fields in CHART
/// and CLABEL lines. If a value contains a literal single quote, it breaks
/// the agent's parser. There is no escape mechanism, so we replace `'` with `"`.
struct SanitizedQuote<'a>(&'a str);

impl fmt::Display for SanitizedQuote<'_> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for ch in self.0.chars() {
            if ch == '\'' {
                f.write_char('"')?;
            } else {
                f.write_char(ch)?;
            }
        }
        Ok(())
    }
}

/// A Netdata chart definition (CHART + CLABEL + DIMENSION block).
pub struct ChartDefinition {
    pub chart_name: String,
    pub title: String,
    pub units: String,
    pub family: String,
    pub context: String,
    pub chart_type: ChartType,
    pub update_every: u64,
    pub labels: Vec<(String, String)>,
    pub dimensions: Vec<String>,
}

impl fmt::Display for ChartDefinition {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        writeln!(
            f,
            "CHART {} '' '{}' '{}' '{}' '{}' {} 1 {} 'store_first'",
            self.chart_name,
            SanitizedQuote(&self.title),
            SanitizedQuote(&self.units),
            SanitizedQuote(&self.family),
            SanitizedQuote(&self.context),
            self.chart_type,
            self.update_every,
        )?;

        if !self.labels.is_empty() {
            for (key, value) in &self.labels {
                writeln!(
                    f,
                    "CLABEL '{}' '{}' 1",
                    SanitizedQuote(key),
                    SanitizedQuote(value),
                )?;
            }
            writeln!(f, "CLABEL_COMMIT")?;
        }

        for dim_name in &self.dimensions {
            writeln!(
                f,
                "DIMENSION {} {} absolute 1 {}",
                dim_name, dim_name, PRECISION_DIVISOR,
            )?;
        }

        Ok(())
    }
}

impl ChartDefinition {
    /// Sort dimensions numerically (ascending), treating "+Inf" as infinity.
    ///
    /// Used for heatmap charts where Netdata preserves the plugin-defined
    /// dimension order and the dashboard renders buckets in that order.
    pub fn sort_dimensions_numerically(&mut self) {
        self.dimensions.sort_by(|a, b| {
            let a_val = if a == "+Inf" {
                f64::INFINITY
            } else {
                a.parse::<f64>().unwrap_or(f64::INFINITY)
            };
            let b_val = if b == "+Inf" {
                f64::INFINITY
            } else {
                b.parse::<f64>().unwrap_or(f64::INFINITY)
            };
            a_val
                .partial_cmp(&b_val)
                .unwrap_or(std::cmp::Ordering::Equal)
        });
    }
}

/// A dimension value ready for output.
#[derive(Debug)]
pub struct DimensionValue {
    pub name: String,
    pub value: Option<f64>,
}

/// Write a data slot (BEGIN + SET for each dimension + END).
///
/// `update_every` is the collection interval in seconds.
/// `slot_timestamp` is the slot-start boundary (floored to `update_every`).
///
/// BEGIN receives the interval converted to microseconds.
/// END receives `slot_timestamp + update_every` — the slot-end boundary —
/// because Netdata interprets a data point at time T as covering
/// `[T - update_every, T]`.
pub fn write_data_slot(
    f: &mut impl fmt::Write,
    chart_name: &str,
    update_every: u64,
    slot_timestamp: u64,
    dimensions: &[DimensionValue],
) -> fmt::Result {
    writeln!(f, "BEGIN {} {}", chart_name, update_every * 1_000_000)?;

    for dim in dimensions {
        match dim.value {
            Some(v) => {
                let scaled = (v * PRECISION_DIVISOR as f64) as i64;
                writeln!(f, "SET {} = {}", dim.name, scaled)?;
            }
            None => writeln!(f, "SET {} = U", dim.name)?,
        }
    }

    writeln!(f, "END {}", slot_timestamp + update_every)?;
    Ok(())
}
