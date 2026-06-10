// SPDX-License-Identifier: GPL-3.0-or-later
//
// PromQL matchers and FFI translation.
//
// The shim accepts `(name, op, value)` triples and applies EQ/NE during
// enumeration. RE/NRE matchers cross the boundary unchanged -- the shim
// returns candidates and the Rust side filters with compiled regexes from
// a process-wide cache. Compiling a regex costs orders of magnitude more
// than executing one, so the cache is load-bearing for repeated queries.

use regex::Regex;
use std::collections::HashMap;
use std::ffi::CString;
use std::sync::{Arc, Mutex, OnceLock};

use super::raw;

/// One PromQL label matcher.
///
/// `Eq`/`Ne` evaluate inside the shim. `Re`/`Nre` carry both the source
/// pattern (passed to the shim verbatim, which ignores it) and the compiled
/// regex used for post-resolution filtering.
#[derive(Debug, Clone)]
pub enum Matcher {
    Eq {
        name: String,
        value: String,
    },
    Ne {
        name: String,
        value: String,
    },
    Re {
        name: String,
        pattern: String,
        compiled: Arc<Regex>,
    },
    Nre {
        name: String,
        pattern: String,
        compiled: Arc<Regex>,
    },
}

impl Matcher {
    pub fn eq(name: impl Into<String>, value: impl Into<String>) -> Self {
        Matcher::Eq {
            name: name.into(),
            value: value.into(),
        }
    }

    pub fn ne(name: impl Into<String>, value: impl Into<String>) -> Self {
        Matcher::Ne {
            name: name.into(),
            value: value.into(),
        }
    }

    pub fn re(name: impl Into<String>, pattern: impl Into<String>) -> Result<Self, regex::Error> {
        let pattern = pattern.into();
        let compiled = compile_regex(&pattern)?;
        Ok(Matcher::Re {
            name: name.into(),
            pattern,
            compiled,
        })
    }

    pub fn nre(name: impl Into<String>, pattern: impl Into<String>) -> Result<Self, regex::Error> {
        let pattern = pattern.into();
        let compiled = compile_regex(&pattern)?;
        Ok(Matcher::Nre {
            name: name.into(),
            pattern,
            compiled,
        })
    }

    pub fn name(&self) -> &str {
        match self {
            Matcher::Eq { name, .. }
            | Matcher::Ne { name, .. }
            | Matcher::Re { name, .. }
            | Matcher::Nre { name, .. } => name,
        }
    }

    /// True if `value` satisfies the matcher.
    pub fn matches(&self, value: &str) -> bool {
        match self {
            Matcher::Eq { value: v, .. } => value == v,
            Matcher::Ne { value: v, .. } => value != v,
            // Prometheus regex matching is full-string by default: the
            // engine anchors at both ends. `Regex::is_match` is a substring
            // check, so we approximate the anchored semantics here.
            Matcher::Re { compiled, .. } => is_full_match(compiled, value),
            Matcher::Nre { compiled, .. } => !is_full_match(compiled, value),
        }
    }

    pub fn is_regex(&self) -> bool {
        matches!(self, Matcher::Re { .. } | Matcher::Nre { .. })
    }
}

fn is_full_match(r: &Regex, value: &str) -> bool {
    // Prometheus' label-matcher regex syntax is anchored implicitly. Our
    // `Regex::Pattern` was compiled from the user-supplied source; we
    // emulate anchoring by checking that a match exists and spans the whole
    // string. `find` returns the leftmost match; we accept iff it covers
    // [0, value.len()).
    if let Some(m) = r.find(value) {
        m.start() == 0 && m.end() == value.len()
    } else {
        false
    }
}

/// Compile (and cache) a Prometheus-style label-matcher regex.
///
/// The compiled regex is returned as an `Arc<Regex>`; the cache holds a
/// strong reference indefinitely. Cache hits are O(1); misses pay one
/// `Regex::new` and one allocation.
pub fn compile_regex(pattern: &str) -> Result<Arc<Regex>, regex::Error> {
    let cache = regex_cache();
    {
        let guard = cache.lock().expect("regex cache poisoned");
        if let Some(r) = guard.get(pattern) {
            return Ok(Arc::clone(r));
        }
    }
    let compiled = Arc::new(Regex::new(pattern)?);
    let mut guard = cache.lock().expect("regex cache poisoned");
    // Recheck after taking the write lock in case another thread inserted.
    if let Some(r) = guard.get(pattern) {
        return Ok(Arc::clone(r));
    }
    guard.insert(pattern.to_string(), Arc::clone(&compiled));
    Ok(compiled)
}

fn regex_cache() -> &'static Mutex<HashMap<String, Arc<Regex>>> {
    static CACHE: OnceLock<Mutex<HashMap<String, Arc<Regex>>>> = OnceLock::new();
    CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

/// Owned backing storage for FFI matcher arrays. The `CString`s here are
/// the lifetime anchors; the matcher pointers inside `Self::ffi` reference
/// them.
pub(crate) struct MatchersFfi {
    _names: Vec<CString>,
    _values: Vec<CString>,
    ffi: Vec<raw::nd_pds_matcher>,
}

impl MatchersFfi {
    /// Build an FFI-shaped matcher array from a slice of Rust matchers.
    ///
    /// The C side sees every matcher (including RE/NRE) so that the shim
    /// can short-circuit obvious mismatches on `__name__`. The shim itself
    /// only evaluates EQ/NE during enumeration; RE/NRE pass through.
    pub fn from_slice(matchers: &[Matcher]) -> Result<Self, MatcherError> {
        let mut names = Vec::with_capacity(matchers.len());
        let mut values = Vec::with_capacity(matchers.len());
        let mut ffi = Vec::with_capacity(matchers.len());

        for m in matchers {
            let (name, op, value) = match m {
                Matcher::Eq { name, value } => (
                    name.as_str(),
                    raw::nd_pds_match_op::ND_PDS_EQ,
                    value.as_str(),
                ),
                Matcher::Ne { name, value } => (
                    name.as_str(),
                    raw::nd_pds_match_op::ND_PDS_NE,
                    value.as_str(),
                ),
                Matcher::Re { name, pattern, .. } => (
                    name.as_str(),
                    raw::nd_pds_match_op::ND_PDS_RE,
                    pattern.as_str(),
                ),
                Matcher::Nre { name, pattern, .. } => (
                    name.as_str(),
                    raw::nd_pds_match_op::ND_PDS_NRE,
                    pattern.as_str(),
                ),
            };

            let name_c = CString::new(name).map_err(|_| MatcherError::NulInLabel)?;
            let value_c = CString::new(value).map_err(|_| MatcherError::NulInLabel)?;

            ffi.push(raw::nd_pds_matcher {
                name: name_c.as_ptr(),
                op: op as i32,
                value: value_c.as_ptr(),
            });

            names.push(name_c);
            values.push(value_c);
        }

        Ok(Self {
            _names: names,
            _values: values,
            ffi,
        })
    }

    pub fn as_ptr(&self) -> *const raw::nd_pds_matcher {
        self.ffi.as_ptr()
    }

    pub fn len(&self) -> usize {
        self.ffi.len()
    }
}

#[derive(Debug, thiserror::Error)]
pub enum MatcherError {
    #[error("label name or value contains an interior NUL byte")]
    NulInLabel,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn eq_matches() {
        let m = Matcher::eq("foo", "bar");
        assert!(m.matches("bar"));
        assert!(!m.matches("baz"));
    }

    #[test]
    fn ne_matches() {
        let m = Matcher::ne("foo", "bar");
        assert!(!m.matches("bar"));
        assert!(m.matches("baz"));
    }

    #[test]
    fn re_is_anchored() {
        let m = Matcher::re("foo", "ba.").unwrap();
        assert!(m.matches("bar"));
        assert!(m.matches("baz"));
        assert!(!m.matches("rebar")); // not anchored at start
        assert!(!m.matches("baseball")); // not anchored at end
    }

    #[test]
    fn nre_is_negation_of_anchored_match() {
        let m = Matcher::nre("foo", "ba.").unwrap();
        assert!(!m.matches("bar"));
        assert!(m.matches("rebar"));
    }

    #[test]
    fn regex_cache_reuses_compilation() {
        let a = compile_regex("abc.*xyz").unwrap();
        let b = compile_regex("abc.*xyz").unwrap();
        assert!(Arc::ptr_eq(&a, &b));
    }

    #[test]
    fn ffi_translation_carries_op_values() {
        let matchers = vec![
            Matcher::eq("a", "1"),
            Matcher::ne("b", "2"),
            Matcher::re("c", "x.*").unwrap(),
            Matcher::nre("d", "y.*").unwrap(),
        ];
        let ffi = MatchersFfi::from_slice(&matchers).unwrap();
        assert_eq!(ffi.len(), 4);
        unsafe {
            let slice = std::slice::from_raw_parts(ffi.as_ptr(), ffi.len());
            assert_eq!(slice[0].op, raw::nd_pds_match_op::ND_PDS_EQ as i32);
            assert_eq!(slice[1].op, raw::nd_pds_match_op::ND_PDS_NE as i32);
            assert_eq!(slice[2].op, raw::nd_pds_match_op::ND_PDS_RE as i32);
            assert_eq!(slice[3].op, raw::nd_pds_match_op::ND_PDS_NRE as i32);
        }
    }

    #[test]
    fn ffi_rejects_nul_in_label() {
        let m = Matcher::eq("a\0b", "v");
        let r = MatchersFfi::from_slice(&[m]);
        assert!(r.is_err());
    }
}
