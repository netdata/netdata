use regex::Regex;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

#[derive(Default, Debug)]
pub struct RegexCache {
    cache: Arc<Mutex<HashMap<String, Regex>>>,
}

impl RegexCache {
    pub fn get(&self, pattern: &str) -> Result<Regex, regex::Error> {
        let mut cache = self.cache.lock().unwrap();

        if let Some(regex) = cache.get(pattern) {
            return Ok(regex.clone());
        }

        let compiled_regex = Regex::new(pattern)?;
        cache.insert(pattern.to_string(), compiled_regex.clone());
        Ok(compiled_regex)
    }
}
