use super::index::FileIndex;
use roaring::RoaringBitmap;

#[derive(Clone, Debug)]
pub enum IndexFilterExpr {
    None,
    Match(RoaringBitmap),
    Conjunction(Vec<IndexFilterExpr>),
    Disjunction(Vec<IndexFilterExpr>),
}

impl IndexFilterExpr {
    /// Get all entry indices that match this filter expression
    pub fn matching_indices(&self) -> RoaringBitmap {
        match self {
            IndexFilterExpr::None => RoaringBitmap::new(),
            IndexFilterExpr::Match(bitmap) => bitmap.clone(),
            IndexFilterExpr::Conjunction(filter_exprs) => {
                if filter_exprs.is_empty() {
                    return RoaringBitmap::new();
                }

                let mut result = filter_exprs[0].matching_indices();
                for expr in filter_exprs.iter().skip(1) {
                    result &= expr.matching_indices();
                    if result.is_empty() {
                        break; // Early termination for empty conjunction
                    }
                }
                result
            }
            IndexFilterExpr::Disjunction(filter_exprs) => {
                let mut result = RoaringBitmap::new();
                for expr in filter_exprs.iter() {
                    result |= expr.matching_indices();
                }
                result
            }
        }
    }

    /// Count the number of matching entries
    pub fn count(&self) -> u64 {
        self.matching_indices().len()
    }

    /// Check if there are any matching entries
    pub fn has_matches(&self) -> bool {
        match self {
            IndexFilterExpr::None => false,
            IndexFilterExpr::Match(bitmap) => !bitmap.is_empty(),
            IndexFilterExpr::Conjunction(filter_exprs) => {
                filter_exprs.iter().all(|expr| expr.has_matches())
            }
            IndexFilterExpr::Disjunction(filter_exprs) => {
                filter_exprs.iter().any(|expr| expr.has_matches())
            }
        }
    }

    /// Get matching indices within a specific range
    pub fn matching_indices_in_range(&self, start: u32, end: u32) -> RoaringBitmap {
        let mut result = self.matching_indices();
        result.remove_range(..start);
        result.remove_range((end + 1)..);
        result
    }

    /// Get matching indices within specific histogram bucket
    pub fn matching_indices_in_bucket(
        &self,
        file_index: &FileIndex,
        bucket_index: usize,
    ) -> Option<RoaringBitmap> {
        let (start, end) = file_index.file_histogram.get_entry_range(bucket_index)?;
        Some(self.matching_indices_in_range(start, end))
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogicalOp {
    Conjunction,
    Disjunction,
}

#[derive(Debug)]
pub struct IndexFilter {
    filter_expr: Option<IndexFilterExpr>,
    current_matches: Vec<String>,
    current_op: LogicalOp,
}

impl Default for IndexFilter {
    fn default() -> Self {
        Self {
            filter_expr: None,
            current_matches: Vec::new(),
            current_op: LogicalOp::Conjunction,
        }
    }
}

impl IndexFilter {
    /// Create a new empty filter
    pub fn new() -> Self {
        Self::default()
    }

    /// Extract the field key from a field=value pair
    fn extract_key(field_value: &str) -> Option<&str> {
        field_value.split('=').next()
    }

    /// Convert current matches to a filter expression
    fn convert_current_matches(&mut self, file_index: &FileIndex) -> Option<IndexFilterExpr> {
        if self.current_matches.is_empty() {
            return None;
        }

        let mut elements = Vec::new();
        let mut i = 0;

        // Sort current matches by key for grouping
        self.current_matches.sort_by(|a, b| {
            let key_a = Self::extract_key(a).unwrap_or("");
            let key_b = Self::extract_key(b).unwrap_or("");
            key_a.cmp(key_b)
        });

        while i < self.current_matches.len() {
            let current_key = Self::extract_key(&self.current_matches[i]).unwrap_or("");
            let start = i;

            // Find all matches with the same key
            while i < self.current_matches.len()
                && Self::extract_key(&self.current_matches[i]).unwrap_or("") == current_key
            {
                i += 1;
            }

            // If we have multiple values for this key, create a disjunction
            if i - start > 1 {
                let mut matches = Vec::with_capacity(i - start);
                for idx in start..i {
                    let field_value = &self.current_matches[idx];
                    if let Some(bitmap) = file_index.entries_index.get(field_value) {
                        matches.push(IndexFilterExpr::Match(bitmap.clone()));
                    } else {
                        matches.push(IndexFilterExpr::None);
                    }
                }
                elements.push(IndexFilterExpr::Disjunction(matches));
            } else {
                let field_value = &self.current_matches[start];
                if let Some(bitmap) = file_index.entries_index.get(field_value) {
                    elements.push(IndexFilterExpr::Match(bitmap.clone()));
                } else {
                    elements.push(IndexFilterExpr::None);
                }
            }
        }

        self.current_matches.clear();

        match elements.len() {
            0 => None,
            1 => Some(elements.into_iter().next().unwrap()),
            _ => Some(IndexFilterExpr::Conjunction(elements)),
        }
    }

    /// Add a field=value match to the current filter being built
    ///
    /// # Examples
    /// ```
    /// filter.add_match("_SYSTEMD_UNIT=ssh.service");
    /// filter.add_match("PRIORITY=6");
    /// ```
    pub fn add_match(&mut self, field_value: &str) {
        if field_value.contains('=') {
            // Insert in sorted order to group by field name
            let key = Self::extract_key(field_value).unwrap_or("");
            let pos = self
                .current_matches
                .binary_search_by(|item| {
                    let item_key = Self::extract_key(item).unwrap_or("");
                    item_key.cmp(key)
                })
                .unwrap_or_else(|e| e);

            self.current_matches.insert(pos, field_value.to_string());
        }
    }

    /// Set the logical operation for combining the current matches with the existing filter
    pub fn set_operation(&mut self, file_index: &FileIndex, op: LogicalOp) {
        let new_expr = self.convert_current_matches(file_index);
        if new_expr.is_none() {
            self.current_op = op;
            return;
        }

        if self.filter_expr.is_none() {
            self.filter_expr = new_expr;
            self.current_op = op;
            return;
        }

        let new_expr = new_expr.unwrap();
        let current_expr = self.filter_expr.take().unwrap();

        self.filter_expr = Some(match (current_expr, self.current_op) {
            (IndexFilterExpr::Disjunction(mut exprs), LogicalOp::Disjunction) => {
                exprs.push(new_expr);
                IndexFilterExpr::Disjunction(exprs)
            }
            (IndexFilterExpr::Conjunction(mut exprs), LogicalOp::Conjunction) => {
                exprs.push(new_expr);
                IndexFilterExpr::Conjunction(exprs)
            }
            (current_expr, LogicalOp::Disjunction) => {
                IndexFilterExpr::Disjunction(vec![current_expr, new_expr])
            }
            (current_expr, LogicalOp::Conjunction) => {
                IndexFilterExpr::Conjunction(vec![current_expr, new_expr])
            }
        });

        self.current_op = op;
    }

    /// Add conjunction (AND) operation
    pub fn add_conjunction(&mut self, file_index: &FileIndex) {
        self.set_operation(file_index, LogicalOp::Conjunction);
    }

    /// Add disjunction (OR) operation
    pub fn add_disjunction(&mut self, file_index: &FileIndex) {
        self.set_operation(file_index, LogicalOp::Disjunction);
    }

    /// Build the final filter expression
    pub fn build(&mut self, file_index: &FileIndex) -> IndexFilterExpr {
        self.set_operation(file_index, self.current_op);

        self.current_matches.clear();
        self.current_op = LogicalOp::Conjunction;
        self.filter_expr.take().unwrap_or(IndexFilterExpr::None)
    }

    /// Convenience method to create a simple match filter
    pub fn simple_match(file_index: &FileIndex, field_value: &str) -> IndexFilterExpr {
        if let Some(bitmap) = file_index.entries_index.get(field_value) {
            IndexFilterExpr::Match(bitmap.clone())
        } else {
            IndexFilterExpr::None
        }
    }

    /// Convenience method to create a conjunction of multiple field=value pairs
    pub fn conjunction(file_index: &FileIndex, field_values: &[&str]) -> IndexFilterExpr {
        let mut filter = IndexFilter::new();
        for field_value in field_values {
            filter.add_match(field_value);
        }
        filter.build(file_index)
    }

    /// Convenience method to create a disjunction of multiple field=value pairs
    pub fn disjunction(file_index: &FileIndex, field_values: &[&str]) -> IndexFilterExpr {
        let matches: Vec<_> = field_values
            .iter()
            .map(|fv| Self::simple_match(file_index, fv))
            .collect();

        if matches.is_empty() {
            IndexFilterExpr::None
        } else if matches.len() == 1 {
            matches.into_iter().next().unwrap()
        } else {
            IndexFilterExpr::Disjunction(matches)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::index::{FileHistogram, FileIndex};

    fn create_test_file_index() -> FileIndex {
        let mut entry_indices = FxHashMap::default();

        // Add some test data
        entry_indices.insert(
            "_SYSTEMD_UNIT=ssh.service".to_string(),
            RoaringBitmap::from_sorted_iter([1, 3, 5, 7]).unwrap(),
        );
        entry_indices.insert(
            "_SYSTEMD_UNIT=nginx.service".to_string(),
            RoaringBitmap::from_sorted_iter([2, 4, 6, 8]).unwrap(),
        );
        entry_indices.insert(
            "PRIORITY=6".to_string(),
            RoaringBitmap::from_sorted_iter([1, 2, 9, 10]).unwrap(),
        );
        entry_indices.insert(
            "PRIORITY=3".to_string(),
            RoaringBitmap::from_sorted_iter([3, 4, 5]).unwrap(),
        );

        FileIndex {
            file_histogram: FileHistogram::default(),
            entries_index: entry_indices,
        }
    }

    #[test]
    fn test_simple_match() {
        let file_index = create_test_file_index();
        let filter = IndexFilter::simple_match(&file_index, "_SYSTEMD_UNIT=ssh.service");

        let matches = filter.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1, 3, 5, 7]);
    }

    #[test]
    fn test_conjunction() {
        let file_index = create_test_file_index();
        let filter =
            IndexFilter::conjunction(&file_index, &["_SYSTEMD_UNIT=ssh.service", "PRIORITY=6"]);

        let matches = filter.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1]); // Only entry 1 matches both
    }

    #[test]
    fn test_disjunction() {
        let file_index = create_test_file_index();
        let filter = IndexFilter::disjunction(
            &file_index,
            &["_SYSTEMD_UNIT=ssh.service", "_SYSTEMD_UNIT=nginx.service"],
        );

        let matches = filter.matching_indices();
        assert_eq!(
            matches.iter().collect::<Vec<_>>(),
            vec![1, 2, 3, 4, 5, 6, 7, 8]
        );
    }

    #[test]
    fn test_complex_filter() {
        let file_index = create_test_file_index();
        let mut filter = IndexFilter::new();

        // Add matches for same key (will be OR'd)
        filter.add_match("_SYSTEMD_UNIT=ssh.service");
        filter.add_match("_SYSTEMD_UNIT=nginx.service");
        filter.add_conjunction(&file_index);

        // Add another condition (will be AND'd with above)
        filter.add_match("PRIORITY=6");

        let result = filter.build(&file_index);
        let matches = result.matching_indices();
        assert_eq!(matches.iter().collect::<Vec<_>>(), vec![1, 2]); // Entries that match PRIORITY=6 AND (ssh OR nginx)
    }
}
