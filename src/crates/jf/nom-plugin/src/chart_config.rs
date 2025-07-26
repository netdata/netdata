// src/chart_config.rs
use regex::Regex;
use serde_json::{Map as JsonMap, Value as JsonValue};
use std::collections::HashMap;

#[derive(Debug, Clone)]
pub struct ChartConfig {
    pub instrumentation_scope_name: Option<Regex>,
    pub instrumentation_scope_version: Option<Regex>,
    pub metric_name: Regex,
    pub chart_instance_pattern: Option<String>,
    pub dimension_name: Option<String>,
    pub priority: u32,
}

impl ChartConfig {
    pub fn new(
        scope_name: Option<&str>,
        scope_version: Option<&str>,
        metric_name: &str,
        chart_instance_pattern: Option<&str>,
        dimension_name: Option<&str>,
        priority: u32,
    ) -> Result<Self, regex::Error> {
        let instrumentation_scope_name = match scope_name {
            Some(pattern) => Some(Regex::new(pattern)?),
            None => None,
        };

        let instrumentation_scope_version = match scope_version {
            Some(pattern) => Some(Regex::new(pattern)?),
            None => None,
        };

        let metric_name = Regex::new(metric_name)?;

        Ok(ChartConfig {
            instrumentation_scope_name,
            instrumentation_scope_version,
            metric_name,
            chart_instance_pattern: chart_instance_pattern.map(String::from),
            dimension_name: dimension_name.map(String::from),
            priority,
        })
    }

    pub fn matches(&self, json_map: &JsonMap<String, JsonValue>) -> bool {
        // Check scope name
        if let Some(scope_regex) = &self.instrumentation_scope_name {
            if let Some(JsonValue::String(scope_name)) = json_map.get("scope.name") {
                if !scope_regex.is_match(scope_name) {
                    return false;
                }
            } else {
                return false;
            }
        }

        // Check scope version
        if let Some(version_regex) = &self.instrumentation_scope_version {
            if let Some(JsonValue::String(scope_version)) = json_map.get("scope.version") {
                if !version_regex.is_match(scope_version) {
                    return false;
                }
            } else {
                return false;
            }
        }

        // Check metric name
        if let Some(JsonValue::String(metric_name)) = json_map.get("metric.name") {
            self.metric_name.is_match(metric_name)
        } else {
            false
        }
    }
}

#[derive(Debug, Default)]
pub struct ChartConfigManager {
    configs: Vec<ChartConfig>,
}

impl ChartConfigManager {
    pub fn new() -> Self {
        Self {
            configs: Vec::new(),
        }
    }

    pub fn with_default_configs() -> Self {
        let mut manager = Self::new();
        manager.add_default_configs();
        manager
    }

    pub fn add_config(&mut self, config: ChartConfig) {
        self.configs.push(config);
        // Sort by priority (higher priority first)
        self.configs.sort_by(|a, b| b.priority.cmp(&a.priority));
    }

    pub fn find_matching_config(
        &self,
        json_map: &JsonMap<String, JsonValue>,
    ) -> Option<&ChartConfig> {
        self.configs.iter().find(|config| config.matches(json_map))
    }

    fn add_default_configs(&mut self) {
        /*
         * network scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*networkscraper$"),
            None,
            r"system\.network\.connections",
            Some("metric.attributes.protocol"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*networkscraper$"),
            None,
            r"system\.network\.dropped",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*networkscraper$"),
            None,
            r"system\.network\.errors",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*networkscraper$"),
            None,
            r"system\.network\.io",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*networkscraper$"),
            None,
            r"system\.network\.packets",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        /*
         * cpu scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*cpuscraper$"),
            None,
            r"system\.cpu\.time",
            Some("metric.attributes.cpu"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*cpuscraper$"),
            None,
            r"system\.cpu\.frequency",
            Some("metric.attributes.cpu"),
            None,
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*cpuscraper$"),
            None,
            r"system\.cpu\.utilization",
            Some("metric.attributes.cpu"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        /*
         * disk scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.io",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.io_time",
            Some("metric.attributes.device"),
            None,
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.merged",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.operation_time",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.operations",
            Some("metric.attributes.device"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.pending_operations",
            Some("metric.attributes.device"),
            None,
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*diskscraper$"),
            None,
            r"system\.disk\.weighted_io",
            Some("metric.attributes.device"),
            None,
            100,
        ) {
            self.add_config(config);
        }

        /*
         * filesystem scraper
         */

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*filesystemscraper$"),
            None,
            r"system\.filesystem\.inodes\.usage",
            Some("metric.attributes.mountpoint"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*filesystemscraper$"),
            None,
            r"system\.filesystem\.usage",
            Some("metric.attributes.mountpoint"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*filesystemscraper$"),
            None,
            r"system\.filesystem\.utilization",
            Some("metric.attributes.mountpoint"),
            None,
            100,
        ) {
            self.add_config(config);
        }

        /*
         * memory scraper
         */

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*memoryscraper$"),
            None,
            r"system\.memory\.utilization",
            None,
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        /*
         * paging scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*pagingscraper$"),
            None,
            r"system\.paging\.faults",
            None,
            Some("metric.attributes.type"),
            100,
        ) {
            self.add_config(config);
        }

        // TODO: should we swap chart instance with dimension?
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*pagingscraper$"),
            None,
            r"system\.paging\.operations",
            Some("metric.attributes.type"),
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*pagingscraper$"),
            None,
            r"system\.paging\.usage",
            Some("metric.attributes.device"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*pagingscraper$"),
            None,
            r"system\.paging\.utilization",
            Some("metric.attributes.device"),
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        /*
         * paging scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processesscraper$"),
            None,
            r"system\.processes\.count",
            None,
            Some("metric.attributes.status"),
            100,
        ) {
            self.add_config(config);
        }

        /*
         * process scraper
         */
        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.cpu\.time",
            None,
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.disk\.io",
            None,
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.context_switches",
            None,
            Some("metric.attributes.type"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.cpu\.utilization",
            None,
            Some("metric.attributes.state"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.disk\.operations",
            None,
            Some("metric.attributes.direction"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.paging\.faults",
            None,
            Some("metric.attributes.type"),
            100,
        ) {
            self.add_config(config);
        }

        if let Ok(config) = ChartConfig::new(
            Some(".*hostmetricsreceiver.*processscraper$"),
            None,
            r"process\.paging\.faults",
            None,
            Some("metric.attributes.type"),
            100,
        ) {
            self.add_config(config);
        }
    }
}
