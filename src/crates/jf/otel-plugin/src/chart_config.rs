use anyhow::{Context, Result};
use regex::Regex;
use serde::{Deserialize, Serialize};
use serde_json::{Map as JsonMap, Value as JsonValue};
use std::fs;
use std::path::Path;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SelectCriteria {
    #[serde(with = "serde_regex", skip_serializing_if = "Option::is_none", default)]
    pub instrumentation_scope_name: Option<Regex>,

    #[serde(with = "serde_regex", skip_serializing_if = "Option::is_none", default)]
    pub instrumentation_scope_version: Option<Regex>,

    #[serde(with = "serde_regex")]
    pub metric_name: Regex,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExtractPattern {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chart_instance_pattern: Option<String>,

    #[serde(skip_serializing_if = "Option::is_none")]
    pub dimension_name: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChartConfig {
    pub select: SelectCriteria,
    pub extract: ExtractPattern,
}

impl ChartConfig {
    pub fn matches(&self, json_map: &JsonMap<String, JsonValue>) -> bool {
        if let Some(scope_regex) = &self.select.instrumentation_scope_name {
            if let Some(JsonValue::String(scope_name)) = json_map.get("scope.name") {
                if !scope_regex.is_match(scope_name) {
                    return false;
                }
            } else {
                return false;
            }
        }

        if let Some(version_regex) = &self.select.instrumentation_scope_version {
            if let Some(JsonValue::String(scope_version)) = json_map.get("scope.version") {
                if !version_regex.is_match(scope_version) {
                    return false;
                }
            } else {
                return false;
            }
        }

        if let Some(JsonValue::String(metric_name)) = json_map.get("metric.name") {
            self.select.metric_name.is_match(metric_name)
        } else {
            false
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ChartConfigs {
    configs: Vec<ChartConfig>,
}

#[derive(Debug, Default, Clone)]
pub struct ChartConfigManager {
    stock: ChartConfigs,
    user: ChartConfigs,
}

impl ChartConfigManager {
    pub fn with_default_configs() -> Self {
        let mut manager = Self::default();
        manager.load_stock_config();
        manager
    }

    pub fn find_matching_config(
        &self,
        json_map: &JsonMap<String, JsonValue>,
    ) -> Option<&ChartConfig> {
        // Chaining-order is important. We want to priority user configurations
        // and fall back to stock configurations if they are missing.
        self.user
            .configs
            .iter()
            .chain(self.stock.configs.iter())
            .find(|config| config.matches(json_map))
    }

    fn load_stock_config(&mut self) {
        const DEFAULT_CONFIGS_YAML: &str =
            include_str!("../configs/otel.d/v1/metrics/hostmetrics-receiver.yml");

        match serde_yaml::from_str::<ChartConfigs>(DEFAULT_CONFIGS_YAML) {
            Ok(configs) => {
                self.stock = configs;
            }
            Err(e) => {
                eprintln!("Failed to parse default configs YAML: {}", e);
            }
        }
    }

    pub fn load_user_configs<P: AsRef<Path>>(&mut self, config_dir: P) -> Result<()> {
        // check dir
        let config_path = config_dir.as_ref();
        if !config_path.exists() {
            return Err(anyhow::anyhow!(
                "Configuration directory does not exist: {}",
                config_path.display()
            ));
        }
        if !config_path.is_dir() {
            return Err(anyhow::anyhow!(
                "Configuration path is not a directory: {}",
                config_path.display()
            ));
        }

        // collect the yaml files
        let mut config_files: Vec<_> = std::fs::read_dir(config_path)
            .with_context(|| {
                format!(
                    "Failed to read chart config directory: {}",
                    config_path.display()
                )
            })?
            .filter_map(|entry| {
                let entry = entry.ok()?;
                let path = entry.path();
                if path.is_file()
                    && matches!(
                        path.extension().and_then(|s| s.to_str()),
                        Some("yml" | "yaml")
                    )
                {
                    Some(path)
                } else {
                    None
                }
            })
            .collect();
        config_files.sort();

        // deserialize them
        self.user = ChartConfigs::default();
        for path in config_files {
            match fs::read_to_string(&path) {
                Ok(contents) => match serde_yaml::from_str::<ChartConfigs>(&contents) {
                    Ok(chart_configs) => {
                        self.user.configs.extend(chart_configs.configs);
                    }
                    Err(e) => {
                        eprintln!("Failed to parse YAML file {}: {}", path.display(), e);
                    }
                },
                Err(e) => {
                    eprintln!("Failed to read file {}: {}", path.display(), e);
                }
            }
        }

        Ok(())
    }
}
