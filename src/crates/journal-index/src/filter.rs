use crate::{Bitmap, FieldName, FieldValuePair, FileIndex};
use std::hash::{Hash, Hasher};
use std::sync::Arc;

/// Represents what a filter expression can match against.
///
/// This enum distinguishes between:
/// - Matching a field name (e.g., "PRIORITY" matches any PRIORITY value)
/// - Matching a specific field=value pair (e.g., "PRIORITY=error")
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
enum FilterTarget {
    /// Match any entry that has this field, regardless of value
    Field(FieldName),
    /// Match entries where this specific field=value pair exists
    Pair(FieldValuePair),
}

/// High-level filter expression that operates on field names and field=value pairs.
///
/// This is the primary type used when constructing filters from user queries.
/// Use [`Filter::match_field_name()`] to match any entry with a specific field,
/// or [`Filter::match_field_value_pair()`] to match a specific field=value combination.
///
/// Filters can be combined using [`Filter::and()`] and [`Filter::or()`] for complex queries.
#[derive(Clone, Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Filter {
    inner: Arc<FilterExpr<FilterTarget>>,
}

impl Filter {
    /// Create a filter that matches any entry with the given field name.
    pub fn match_field_name(name: FieldName) -> Self {
        Self {
            inner: Arc::new(FilterExpr::Match(FilterTarget::Field(name))),
        }
    }

    /// Create a filter that matches a specific field=value pair.
    pub fn match_field_value_pair(pair: FieldValuePair) -> Self {
        Self {
            inner: Arc::new(FilterExpr::Match(FilterTarget::Pair(pair))),
        }
    }

    /// Combine multiple filters with AND logic.
    pub fn and(filters: Vec<Self>) -> Self {
        let inner_filters: Vec<FilterExpr<FilterTarget>> =
            filters.into_iter().map(|f| (*f.inner).clone()).collect();

        Self {
            inner: Arc::new(FilterExpr::and(inner_filters)),
        }
    }

    /// Combine multiple filters with OR logic.
    pub fn or(filters: Vec<Self>) -> Self {
        let inner_filters: Vec<FilterExpr<FilterTarget>> =
            filters.into_iter().map(|f| (*f.inner).clone()).collect();

        Self {
            inner: Arc::new(FilterExpr::or(inner_filters)),
        }
    }

    /// Create a filter that matches nothing.
    pub fn none() -> Self {
        Self {
            inner: Arc::new(FilterExpr::None),
        }
    }

    /// Check if this is a None filter.
    pub fn is_none(&self) -> bool {
        matches!(self.inner.as_ref(), FilterExpr::None)
    }

    /// Evaluate this filter against a file index to get matching entry indices.
    pub fn evaluate(&self, file_index: &FileIndex) -> Bitmap {
        self.inner.resolve(file_index).evaluate()
    }
}

impl PartialEq for Filter {
    fn eq(&self, other: &Self) -> bool {
        // Quick pointer equality check first
        if Arc::ptr_eq(&self.inner, &other.inner) {
            return true;
        }

        // Fall back to value equality
        self.inner == other.inner
    }
}

impl Eq for Filter {}

impl std::hash::Hash for Filter {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        self.inner.hash(state);
    }
}

impl std::fmt::Display for Filter {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.inner)
    }
}

#[derive(Clone, Debug, PartialEq)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
enum FilterExpr<T> {
    None,
    Match(T),
    Conjunction(Vec<Self>),
    Disjunction(Vec<Self>),
}

impl Eq for FilterExpr<FilterTarget> {}

impl Hash for FilterExpr<FilterTarget> {
    fn hash<H: Hasher>(&self, state: &mut H) {
        std::mem::discriminant(self).hash(state);

        match self {
            FilterExpr::None => {}
            FilterExpr::Match(target) => target.hash(state),
            FilterExpr::Conjunction(filters) => filters.hash(state),
            FilterExpr::Disjunction(filters) => filters.hash(state),
        }
    }
}

impl FilterExpr<FilterTarget> {
    fn and(filters: Vec<Self>) -> Self {
        // Flatten any nested conjunctions and remove None filters
        let mut flattened = Vec::new();
        for filter in filters {
            match filter {
                FilterExpr::Conjunction(inner) => flattened.extend(inner),
                FilterExpr::None => continue,
                other => flattened.push(other),
            }
        }

        match flattened.len() {
            0 => FilterExpr::None,
            1 => flattened.into_iter().next().unwrap(),
            _ => FilterExpr::Conjunction(flattened),
        }
    }

    fn or(filters: Vec<Self>) -> Self {
        // Flatten any nested disjunctions and remove None filters
        let mut flattened = Vec::new();
        for filter in filters {
            match filter {
                FilterExpr::Disjunction(inner) => flattened.extend(inner),
                FilterExpr::None => continue,
                other => flattened.push(other),
            }
        }

        match flattened.len() {
            0 => FilterExpr::None,
            1 => flattened.into_iter().next().unwrap(),
            _ => FilterExpr::Disjunction(flattened),
        }
    }

    /// Convert a [`FilterExpr<FilterTarget>`] to [`FilterExpr<Bitmap>`] using the file index
    fn resolve(&self, file_index: &FileIndex) -> FilterExpr<Bitmap> {
        match self {
            FilterExpr::None => FilterExpr::None,
            FilterExpr::Match(target) => match target {
                FilterTarget::Field(field_name) => {
                    // Find all field=value pairs with matching field name
                    let matches: Vec<_> = file_index
                        .bitmaps()
                        .iter()
                        .filter(|(pair, _)| pair.field() == field_name.as_str())
                        .map(|(_, bitmap)| FilterExpr::Match(bitmap.clone()))
                        .collect();

                    match matches.len() {
                        0 => FilterExpr::None,
                        1 => matches.into_iter().next().unwrap(),
                        _ => FilterExpr::Disjunction(matches),
                    }
                }
                FilterTarget::Pair(pair) => {
                    // Lookup specific field=value pair
                    if let Some(bitmap) = file_index.bitmaps().get(pair) {
                        FilterExpr::Match(bitmap.clone())
                    } else {
                        FilterExpr::None
                    }
                }
            },
            FilterExpr::Conjunction(filters) => {
                let mut resolved = Vec::with_capacity(filters.len());
                for filter in filters {
                    let r = filter.resolve(file_index);
                    if matches!(r, FilterExpr::None) {
                        return FilterExpr::None;
                    }
                    resolved.push(r);
                }

                match resolved.len() {
                    0 => FilterExpr::None,
                    1 => resolved.into_iter().next().unwrap(),
                    _ => FilterExpr::Conjunction(resolved),
                }
            }
            FilterExpr::Disjunction(filters) => {
                let mut resolved = Vec::with_capacity(filters.len());
                for filter in filters {
                    let r = filter.resolve(file_index);
                    if !matches!(r, FilterExpr::None) {
                        resolved.push(r);
                    }
                }

                match resolved.len() {
                    0 => FilterExpr::None,
                    1 => resolved.into_iter().next().unwrap(),
                    _ => FilterExpr::Disjunction(resolved),
                }
            }
        }
    }
}

impl std::fmt::Display for FilterExpr<FilterTarget> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            FilterExpr::None => write!(f, "None"),
            FilterExpr::Match(target) => match target {
                FilterTarget::Field(name) => write!(f, "{}", name),
                FilterTarget::Pair(pair) => write!(f, "{}", pair),
            },
            FilterExpr::Conjunction(filters) => {
                write!(f, "(")?;
                for (i, filter) in filters.iter().enumerate() {
                    if i > 0 {
                        write!(f, " AND ")?;
                    }
                    write!(f, "{}", filter)?;
                }
                write!(f, ")")
            }
            FilterExpr::Disjunction(filters) => {
                write!(f, "(")?;
                for (i, filter) in filters.iter().enumerate() {
                    if i > 0 {
                        write!(f, " OR ")?;
                    }
                    write!(f, "{}", filter)?;
                }
                write!(f, ")")
            }
        }
    }
}

impl FilterExpr<Bitmap> {
    /// Get all entry indices that match this filter expression
    fn evaluate(&self) -> Bitmap {
        match self {
            Self::None => Bitmap::new(),
            Self::Match(bitmap) => bitmap.clone(),
            Self::Conjunction(filter_exprs) => {
                if filter_exprs.is_empty() {
                    return Bitmap::new();
                }

                let mut result = filter_exprs[0].evaluate();
                for expr in filter_exprs.iter().skip(1) {
                    result &= expr.evaluate();
                    if result.is_empty() {
                        break; // Early termination for empty conjunction
                    }
                }
                result
            }
            Self::Disjunction(filter_exprs) => {
                let mut result = Bitmap::new();
                for expr in filter_exprs.iter() {
                    result |= expr.evaluate();
                }
                result
            }
        }
    }
}
