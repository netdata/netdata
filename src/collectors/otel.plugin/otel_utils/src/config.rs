#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::sync::Arc;

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct MetricConfig {
    #[serde(default, rename = "dimensions_attribute")]
    pub dimension_attribute: Option<String>,

    #[serde(default)]
    pub instance_attributes: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ScopeConfig {
    pub metrics: HashMap<String, Arc<MetricConfig>>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct Config {
    #[serde(rename = "scopes")]
    scope_configs: HashMap<String, Arc<ScopeConfig>>,

    #[serde(skip)]
    patterns: Vec<(String, regex::Regex)>,
}

impl Config {
    pub fn load<P: AsRef<Path>>(path: P) -> Result<Config, Box<dyn std::error::Error>> {
        let contents = fs::read_to_string(path)?;
        let mut config: Config = serde_yaml::from_str(&contents)?;

        for (pattern, _) in config.scope_configs.iter() {
            let re = regex::Regex::new(&format!("^{}$", pattern))?;
            config.patterns.push((pattern.clone(), re));
        }

        Ok(config)
    }

    pub fn scope_config(&self, scope_name: &str) -> Option<Arc<ScopeConfig>> {
        if let Some(s) = self.scope_configs.get(scope_name) {
            return Some(s.clone());
        }

        None
    }

    pub fn match_scope(&mut self, scope_name: &str) -> Option<Arc<ScopeConfig>> {
        for (pattern, re) in self.patterns.iter() {
            if re.is_match(scope_name) {
                if let Some(cfg) = self.scope_configs.get(pattern) {
                    return Some(cfg.clone());
                }
            }
        }

        None
    }

    pub fn insert_scope(&mut self, scope_name: &str, scope_config: Arc<ScopeConfig>) {
        self.scope_configs
            .insert(String::from(scope_name), scope_config);
    }
}
