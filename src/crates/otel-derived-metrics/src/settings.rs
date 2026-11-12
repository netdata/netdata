//! The settings of the derived metrics, and the environment variables that
//! replace them.
//!
//! The variables are an escape hatch, not configuration: the usual values
//! are meant to fit every install, and `otel.yaml` has no section for them.

use std::ffi::OsStr;

use anyhow::Context;

/// How many services have their own charts at once, per signal. The records
/// of further services count in one overflow chart set, until services with
/// their own charts go idle.
pub const MAX_SERVICES: usize = 500;

/// The environment variable that turns the per-service charts off: `true` or
/// `false`.
pub const ENABLED_VARIABLE: &str = "NETDATA_OTEL_DERIVED_METRICS_ENABLED";

/// The environment variable that replaces [`MAX_SERVICES`]: a whole number of
/// at least 1.
pub const MAX_SERVICES_VARIABLE: &str = "NETDATA_OTEL_DERIVED_METRICS_MAX_SERVICES";

/// What [`crate::DerivedMetrics`] is made with.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Settings {
    /// Make the per-service charts.
    pub enabled: bool,
    /// See [`MAX_SERVICES`].
    pub max_services: usize,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            enabled: true,
            max_services: MAX_SERVICES,
        }
    }
}

impl Settings {
    /// The usual settings, each replaced by the value of its environment
    /// variable when that is set ([`ENABLED_VARIABLE`],
    /// [`MAX_SERVICES_VARIABLE`]).
    ///
    /// # Errors
    ///
    /// A value that is not valid, so that a mistyped value cannot leave a
    /// setting at its usual value unnoticed.
    pub fn from_environment() -> anyhow::Result<Self> {
        Self::from_values(
            std::env::var_os(ENABLED_VARIABLE).as_deref(),
            std::env::var_os(MAX_SERVICES_VARIABLE).as_deref(),
        )
    }

    fn from_values(enabled: Option<&OsStr>, max_services: Option<&OsStr>) -> anyhow::Result<Self> {
        let mut settings = Self::default();

        if let Some(value) = enabled {
            settings.enabled = match text(ENABLED_VARIABLE, value)? {
                "true" => true,
                "false" => false,
                other => anyhow::bail!(
                    "invalid value for {ENABLED_VARIABLE}: '{other}': expected true or false"
                ),
            };
        }

        if let Some(value) = max_services {
            let value = text(MAX_SERVICES_VARIABLE, value)?;
            let max_services: usize = value.parse().map_err(|e| {
                anyhow::anyhow!("invalid value for {MAX_SERVICES_VARIABLE}: '{value}': {e}")
            })?;
            anyhow::ensure!(
                max_services >= 1,
                "invalid value for {MAX_SERVICES_VARIABLE}: '{max_services}': expected at least 1"
            );
            settings.max_services = max_services;
        }

        Ok(settings)
    }
}

/// The text that the environment variable `name` holds as `value`.
fn text<'a>(name: &str, value: &'a OsStr) -> anyhow::Result<&'a str> {
    value
        .to_str()
        .with_context(|| format!("{name} contains invalid UTF-8"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn settings(enabled: Option<&str>, max_services: Option<&str>) -> anyhow::Result<Settings> {
        Settings::from_values(enabled.map(OsStr::new), max_services.map(OsStr::new))
    }

    #[test]
    fn the_environment_replaces_the_usual_settings() {
        assert_eq!(settings(None, None).unwrap(), Settings::default());
        assert_eq!(
            settings(Some("false"), Some("20")).unwrap(),
            Settings {
                enabled: false,
                max_services: 20,
            }
        );
        assert_eq!(settings(Some("true"), None).unwrap(), Settings::default());
    }

    #[test]
    fn a_value_that_is_not_valid_is_an_error_that_names_its_variable() {
        for (enabled, max_services, variable) in [
            (Some("no"), None, ENABLED_VARIABLE),
            (Some(""), None, ENABLED_VARIABLE),
            (None, Some("0"), MAX_SERVICES_VARIABLE),
            (None, Some("many"), MAX_SERVICES_VARIABLE),
        ] {
            let error = settings(enabled, max_services).unwrap_err().to_string();
            assert!(
                error.contains(&format!("invalid value for {variable}")),
                "{error}"
            );
        }
    }
}
