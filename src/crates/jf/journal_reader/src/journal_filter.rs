use error::{JournalError, Result};
use journal_file::{
    offset_array::{Direction, InlinedCursor},
    JournalFile,
};
use window_manager::MemoryMap;

#[derive(Clone, Debug)]
pub enum FilterExpr {
    Match(u64, Option<InlinedCursor>),
    Conjunction(Vec<FilterExpr>),
    Disjunction(Vec<FilterExpr>),
}

impl FilterExpr {
    pub fn lookup<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        needle_offset: u64,
        direction: Direction,
    ) -> Result<Option<u64>> {
        let predicate =
            move |entry_offset: u64| -> Result<bool> { Ok(entry_offset < needle_offset) };

        match self {
            FilterExpr::Match(data_offset, _) => {
                let entry_offset = journal_file.data_object_directed_partition_point(
                    *data_offset,
                    predicate,
                    direction,
                )?;
                Ok(entry_offset)
            }
            FilterExpr::Conjunction(filter_exprs) => {
                let mut current_offset = needle_offset;

                loop {
                    let previous_offset = current_offset;

                    for filter_expr in filter_exprs {
                        if direction == Direction::Backward {
                            current_offset = current_offset.saturating_add(1);
                        }

                        match filter_expr.lookup(journal_file, current_offset, direction)? {
                            Some(new_offset) => current_offset = new_offset,
                            None => return Ok(None),
                        }
                    }

                    if current_offset == previous_offset {
                        return Ok(Some(current_offset));
                    }
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                let cmp = match direction {
                    Direction::Forward => std::cmp::min,
                    Direction::Backward => std::cmp::max,
                };

                filter_exprs.iter().try_fold(None, |acc, expr| {
                    let result = expr.lookup(journal_file, needle_offset, direction)?;

                    Ok(match (acc, result) {
                        (None, Some(offset)) => Some(offset),
                        (Some(best), Some(offset)) => Some(cmp(best, offset)),
                        (acc, None) => acc,
                    })
                })
            }
        }
    }

    pub fn head(&mut self) -> &mut Self {
        match self {
            FilterExpr::Match(_, None) => (),
            FilterExpr::Match(_, Some(ic)) => {
                *ic = ic.head();
            }
            FilterExpr::Conjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.head();
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.head();
                }
            }
        }

        self
    }

    pub fn tail<M: MemoryMap>(&mut self, journal_file: &JournalFile<M>) -> Result<&mut Self> {
        match self {
            FilterExpr::Match(_, None) => (),
            FilterExpr::Match(_, Some(ic)) => {
                *ic = ic.tail(journal_file)?;
            }
            FilterExpr::Conjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.tail(journal_file)?;
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                for filter_expr in filter_exprs.iter_mut() {
                    filter_expr.tail(journal_file)?;
                }
            }
        }

        Ok(self)
    }

    // Returns the offset of the next matching entry, if any, with an offset
    // greater or equal to the needle offset.
    pub fn next<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        needle_offset: u64,
    ) -> Result<Option<u64>> {
        match self {
            FilterExpr::Match(_, None) => Ok(None),
            FilterExpr::Match(_, Some(ic)) => ic.next_until(journal_file, needle_offset),
            FilterExpr::Conjunction(filter_exprs) => {
                let mut needle_offset = needle_offset;

                loop {
                    let previous_offset = needle_offset;

                    for fe in filter_exprs.iter_mut() {
                        if let Some(new_offset) = fe.next(journal_file, needle_offset)? {
                            needle_offset = new_offset;
                        } else {
                            return Ok(None);
                        }
                    }

                    if needle_offset == previous_offset {
                        return Ok(Some(needle_offset));
                    }
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                let mut best_offset: Option<u64> = None;

                for fe in filter_exprs.iter_mut() {
                    if let Some(fe_offset) = fe.next(journal_file, needle_offset)? {
                        best_offset = match best_offset {
                            Some(offset) => Some(fe_offset.min(offset)),
                            None => Some(fe_offset),
                        };
                    }
                }

                Ok(best_offset)
            }
        }
    }

    // Returns the offset of the previous matching entry, if any, with an offset
    // less or equal to the needle offset.
    pub fn previous<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        needle_offset: u64,
    ) -> Result<Option<u64>> {
        match self {
            FilterExpr::Match(_, None) => Ok(None),
            FilterExpr::Match(_, Some(ic)) => ic.previous_until(journal_file, needle_offset),
            FilterExpr::Conjunction(filter_exprs) => {
                let mut needle_offset = needle_offset;

                loop {
                    let previous_offset = needle_offset;

                    for fe in filter_exprs.iter_mut().rev() {
                        if let Some(new_offset) = fe.previous(journal_file, needle_offset)? {
                            needle_offset = new_offset;
                        } else {
                            return Ok(None);
                        }
                    }

                    if needle_offset == previous_offset {
                        return Ok(Some(needle_offset));
                    }
                }
            }
            FilterExpr::Disjunction(filter_exprs) => {
                let mut best_offset: Option<u64> = None;

                for fe in filter_exprs.iter_mut() {
                    if let Some(fe_offset) = fe.previous(journal_file, needle_offset)? {
                        best_offset = match best_offset {
                            Some(offset) => Some(fe_offset.max(offset)),
                            None => Some(fe_offset),
                        };
                    }
                }

                Ok(best_offset)
            }
        }
    }

    pub fn dump<M: MemoryMap>(&self, journal_file: &JournalFile<M>) -> Result<String> {
        let mut output = String::new();
        self.dump_internal(journal_file, 0, &mut output)?;
        Ok(output)
    }

    /// Helper function for format_data_objects that handles nested expressions and indentation
    fn dump_internal<M: MemoryMap>(
        &self,
        journal_file: &JournalFile<M>,
        indent_level: usize,
        output: &mut String,
    ) -> Result<()> {
        let indent = "  ".repeat(indent_level);

        match self {
            FilterExpr::Match(data_offset, inlined_cursor) => {
                // Load the data object
                let data_object = journal_file.data_ref(*data_offset)?;

                // Get the payload as a string if possible
                let payload_bytes = data_object.payload_bytes();
                let payload_str = String::from_utf8_lossy(payload_bytes);

                // Format the data offset and payload
                output.push_str(&format!(
                    "{}Match[offset=0x{:x}]: {}\n",
                    indent, data_offset, payload_str
                ));

                // Format cursor information if available
                if let Some(ic) = inlined_cursor {
                    output.push_str(&format!("{}  Cursor: {:?}\n", indent, ic));
                }
            }
            FilterExpr::Conjunction(filter_exprs) => {
                output.push_str(&format!("{}Conjunction (AND) {{\n", indent));
                for expr in filter_exprs {
                    expr.dump_internal(journal_file, indent_level + 1, output)?;
                }
                output.push_str(&format!("{}}}\n", indent));
            }
            FilterExpr::Disjunction(filter_exprs) => {
                output.push_str(&format!("{}Disjunction (OR) {{\n", indent));
                for expr in filter_exprs {
                    expr.dump_internal(journal_file, indent_level + 1, output)?;
                }
                output.push_str(&format!("{}}}\n", indent));
            }
        }

        Ok(())
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogicalOp {
    Conjunction,
    Disjunction,
}

#[derive(Debug)]
pub struct JournalFilter {
    filter_expr: Option<FilterExpr>,
    current_matches: Vec<Vec<u8>>,
    current_op: LogicalOp,
}

impl Default for JournalFilter {
    fn default() -> Self {
        Self {
            filter_expr: None,
            current_matches: Vec::new(),
            current_op: LogicalOp::Conjunction,
        }
    }
}

impl JournalFilter {
    fn extract_key(kv_pair: &[u8]) -> Option<&[u8]> {
        if let Some(equal_pos) = kv_pair.iter().position(|&b| b == b'=') {
            Some(&kv_pair[..equal_pos])
        } else {
            None
        }
    }

    fn convert_current_matches<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
    ) -> Result<Option<FilterExpr>> {
        if self.current_matches.is_empty() {
            return Ok(None);
        }

        let mut elements = Vec::new();
        let mut i = 0;

        while i < self.current_matches.len() {
            let current_key = Self::extract_key(&self.current_matches[i]).unwrap_or(&[]);
            let start = i;

            // Find all matches with the same key
            while i < self.current_matches.len()
                && Self::extract_key(&self.current_matches[i]).unwrap_or(&[]) == current_key
            {
                i += 1;
            }

            // If we have multiple values for this key, create a disjunction
            if i - start > 1 {
                let mut matches = Vec::with_capacity(i - start);
                for idx in start..i {
                    let data = self.current_matches[idx].as_slice();
                    let hash = journal_file.hash(data);
                    let offset = journal_file.find_data_offset_by_payload(data, hash)?;

                    let ic = journal_file.data_ref(offset)?.inlined_cursor();
                    matches.push(FilterExpr::Match(offset, ic));
                }
                elements.push(FilterExpr::Disjunction(matches));
            } else {
                let data = self.current_matches[start].as_slice();
                let hash = journal_file.hash(data);
                let offset = journal_file.find_data_offset_by_payload(data, hash)?;

                let ic = journal_file.data_ref(offset)?.inlined_cursor();
                elements.push(FilterExpr::Match(offset, ic));
            }
        }

        self.current_matches.clear();

        match elements.len() {
            0 => panic!("Could not create filter elements from current matches"),
            1 => Ok(Some(elements.remove(0))),
            _ => Ok(Some(FilterExpr::Conjunction(elements))),
        }
    }

    pub fn add_match(&mut self, kv_pair: &[u8]) {
        if kv_pair.contains(&b'=') {
            let new_item = kv_pair.to_vec();
            let new_key = Self::extract_key(&new_item).unwrap_or(&[]);

            // Find the insertion position using binary search
            let pos = self
                .current_matches
                .binary_search_by(|item| {
                    let key = Self::extract_key(item).unwrap_or(&[]);
                    key.cmp(new_key)
                })
                .unwrap_or_else(|e| e);

            // Insert at the found position
            self.current_matches.insert(pos, new_item);
        }
    }

    pub fn set_operation<M: MemoryMap>(
        &mut self,
        journal_file: &JournalFile<M>,
        op: LogicalOp,
    ) -> Result<()> {
        let new_expr = self.convert_current_matches(journal_file)?;
        if new_expr.is_none() {
            self.current_op = op;
            return Ok(());
        }

        if self.filter_expr.is_none() {
            self.filter_expr = new_expr;
            self.current_op = op;
            return Ok(());
        }

        let new_expr = new_expr.unwrap();
        let current_expr = self.filter_expr.take().unwrap();

        self.filter_expr = Some(match (current_expr, self.current_op) {
            (FilterExpr::Disjunction(mut exprs), LogicalOp::Disjunction) => {
                exprs.push(new_expr);
                FilterExpr::Disjunction(exprs)
            }
            (FilterExpr::Conjunction(mut exprs), LogicalOp::Conjunction) => {
                exprs.push(new_expr);
                FilterExpr::Conjunction(exprs)
            }
            (current_expr, LogicalOp::Disjunction) => {
                FilterExpr::Disjunction(vec![current_expr, new_expr])
            }
            (current_expr, LogicalOp::Conjunction) => {
                FilterExpr::Conjunction(vec![current_expr, new_expr])
            }
        });

        self.current_op = op;
        Ok(())
    }

    pub fn build<M: MemoryMap>(&mut self, journal_file: &JournalFile<M>) -> Result<FilterExpr> {
        self.set_operation(journal_file, self.current_op)?;

        self.current_matches.clear();
        self.current_op = LogicalOp::Conjunction;
        self.filter_expr.take().ok_or(JournalError::MalformedFilter)
    }
}
