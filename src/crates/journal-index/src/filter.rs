//! The query filter over a [`FileIndex`]'s per-pair bitmaps.
//!
//! [`Filter`] is the only public item here: an expression
//! tree of field-name and field=value predicates combined with AND/OR.
//! Predicates are exact string matches - no ranges, globs or regex.
//! Time bounds and free-text regex are separate [`crate::LogQueryParams`]
//! inputs (builder [`crate::LogQueryParamsBuilder::with_regex`])
//! applied per entry after the bitmap stage by
//! [`crate::FileIndex::find_log_entries`].
//! Matching is case-sensitive: names and values are used verbatim, the
//! index normalizes nothing (`src/field_types.rs` `FieldName`/
//! `FieldValuePair` derive the inner string's `Eq`).
//!
//! Evaluation is infallible. `Filter::evaluate` resolves each leaf to
//! its bitmap (`FilterExpr::resolve`) and folds the result into one
//! entry set with `&=`/`|=` (`FilterExpr::evaluate`). An unknown field or
//! pair yields an empty bitmap, never an error. The resulting bits are
//! entry indices into `FileIndex::entry_offsets`, which
//! [`crate::FileIndex::find_log_entries`] turns into file offsets.
//!
//! Consumers: otel-legacy-logs builds one from request selections, ORing
//! the values within a field and ANDing the fields together
//! (`otel-legacy-logs/src/handler.rs` `build_filter_from_selections`);
//! journal-engine applies the
//! same `Filter` per histogram bucket (`journal-engine/src/histogram.rs`)
//! and carries it through per-file query params (`journal-engine/src/
//! logs/query.rs`). `Filter::none()` evaluates to an empty bitmap;
//! callers check `is_none()` and skip applying it (`journal-engine/src/
//! histogram.rs`, `otel-legacy-logs/src/handler.rs`), leaving
//! `LogQueryParams::filter` as `None` for the unfiltered full-coverage
//! path ([`crate::FileIndex::find_log_entries`]).
//!
//! Same-named but unrelated: journal-core/src/file/filter.rs has its own
//! `FilterExpr` (`journal-core/src/file/filter.rs`), and
//! journal-core/src/file/index_filter.rs is an older orphan that
//! file/mod.rs never declares - neither is this type.
use crate::{Bitmap, FieldName, FieldValuePair, FileIndex};
use std::hash::{Hash, Hasher};
use std::sync::Arc;

/// What a `Match` leaf tests: a field's presence or one exact pair.
/// Both match exactly and case-sensitively (the `src/field_types.rs`
/// newtypes compare their inner strings verbatim).
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
enum FilterTarget {
    /// Any entry having this field, whatever the value (a resolve-time
    /// union of the field's pair bitmaps)
    Field(FieldName),
    /// Entries having this exact pair (an exact key lookup in the
    /// pair-bitmap map)
    Pair(FieldValuePair),
}

/// Query filter over indexed field names and field=value pairs - the
/// type callers build from user query selections and evaluate against
/// a [`FileIndex`].
///
/// Cloning is cheap: the expression tree lives behind an `Arc`. Build
/// leaves with [`Filter::match_field_name`] and
/// [`Filter::match_field_value_pair`], combine with [`Filter::and`] and
/// [`Filter::or`], then [`Filter::evaluate`] against a [`FileIndex`] to
/// get the matching entries as a [`Bitmap`].
#[derive(Clone, Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Filter {
    inner: Arc<FilterExpr<FilterTarget>>,
}

impl Filter {
    /// Create a filter matching any entry that has this field, whatever
    /// its value; a field absent from the index matches nothing.
    pub fn match_field_name(name: FieldName) -> Self {
        Self {
            inner: Arc::new(FilterExpr::Match(FilterTarget::Field(name))),
        }
    }

    /// Create a filter matching this exact field=value pair; a pair
    /// absent from the index matches nothing.
    pub fn match_field_value_pair(pair: FieldValuePair) -> Self {
        Self {
            inner: Arc::new(FilterExpr::Match(FilterTarget::Pair(pair))),
        }
    }

    /// Combine filters with AND: an entry matches when every input
    /// matches.
    ///
    /// Nested ANDs flatten and `none()` inputs drop out; no surviving
    /// operands (empty input or all-`none()`) collapse to `none()`.
    pub fn and(filters: Vec<Self>) -> Self {
        let inner_filters: Vec<FilterExpr<FilterTarget>> =
            filters.into_iter().map(|f| (*f.inner).clone()).collect();

        Self {
            inner: Arc::new(FilterExpr::and(inner_filters)),
        }
    }

    /// Combine filters with OR: an entry matches when any input matches.
    ///
    /// Nested ORs flatten and `none()` inputs drop out; no surviving
    /// operands (empty input or all-`none()`) collapse to `none()` - an
    /// OR with no branches matches nothing, not everything.
    pub fn or(filters: Vec<Self>) -> Self {
        let inner_filters: Vec<FilterExpr<FilterTarget>> =
            filters.into_iter().map(|f| (*f.inner).clone()).collect();

        Self {
            inner: Arc::new(FilterExpr::or(inner_filters)),
        }
    }

    /// Create a filter that matches nothing.
    ///
    /// Callers also read this as "no filter": they check
    /// [`Filter::is_none`] and skip applying it (`journal-engine/src/
    /// histogram.rs`, `otel-legacy-logs/src/handler.rs`). Evaluated
    /// directly it yields an empty bitmap; the unfiltered case is
    /// expressed by leaving `LogQueryParams::filter` unset, which gets a
    /// full-coverage bitmap instead ([`FileIndex::find_log_entries`]).
    pub fn none() -> Self {
        Self {
            inner: Arc::new(FilterExpr::None),
        }
    }

    /// Whether this is `none()` - the marker callers use to skip filtering.
    pub fn is_none(&self) -> bool {
        matches!(self.inner.as_ref(), FilterExpr::None)
    }

    /// Evaluate this filter against a file index.
    ///
    /// Returns the matching entries as a [`Bitmap`] of entry indices -
    /// the bits index the file's `entry_offsets` list, which
    /// [`FileIndex::find_log_entries`] maps to file offsets; an
    /// empty bitmap means nothing matched. Infallible: unknown fields and
    /// pairs match nothing rather than erroring.
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

/// The filter expression tree, generic over what a `Match` leaf holds.
///
/// `FilterExpr<FilterTarget>` is the user-facing tree a [`Filter`] wraps;
/// `FilterExpr<Bitmap>` is the resolved tree `FilterExpr::resolve` builds
/// from a [`FileIndex`]. Compounds are never empty and never hold `None`
/// members: `and`/`or` drop and collapse as they build, and so does
/// `resolve`.
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
        // Discriminant + payload: same members in a Conjunction vs a
        // Disjunction hash differently, and operand order is preserved
        // (Vec hashing and the derived Eq both keep call order).
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
        // Flatten nested conjunctions; drop None members (AND identity)
        let mut flattened = Vec::new();
        for filter in filters {
            match filter {
                FilterExpr::Conjunction(inner) => flattened.extend(inner),
                FilterExpr::None => continue,
                other => flattened.push(other),
            }
        }

        // 0 -> None, 1 -> the element itself (no wrapper kept)
        match flattened.len() {
            0 => FilterExpr::None,
            1 => flattened.into_iter().next().unwrap(),
            _ => FilterExpr::Conjunction(flattened),
        }
    }

    fn or(filters: Vec<Self>) -> Self {
        // Flatten nested disjunctions; drop None members (OR identity)
        let mut flattened = Vec::new();
        for filter in filters {
            match filter {
                FilterExpr::Disjunction(inner) => flattened.extend(inner),
                FilterExpr::None => continue,
                other => flattened.push(other),
            }
        }

        // 0 -> None, 1 -> the element itself (no wrapper kept)
        match flattened.len() {
            0 => FilterExpr::None,
            1 => flattened.into_iter().next().unwrap(),
            _ => FilterExpr::Disjunction(flattened),
        }
    }

    /// Resolve every leaf into a `FilterExpr<Bitmap>`: a field to the
    /// union of its pair bitmaps, a pair to its exact bitmap, an absent
    /// field or pair to `None`. Compounds resolve their children - an
    /// AND with a `None` child is `None` outright, an OR skips `None`
    /// children - and single-member results collapse to the member.
    fn resolve(&self, file_index: &FileIndex) -> FilterExpr<Bitmap> {
        match self {
            FilterExpr::None => FilterExpr::None,
            FilterExpr::Match(target) => match target {
                FilterTarget::Field(field_name) => {
                    // Field presence: union of every pair bitmap with this field name
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
                    // Exact key lookup in the pair-bitmap map
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
    /// Fold the resolved tree into one bitmap of matching entry indices:
    /// `None` is empty, `Match` is its bitmap, AND intersects with `&=`,
    /// OR unions with `|=`.
    fn evaluate(&self) -> Bitmap {
        match self {
            Self::None => Bitmap::new(),
            Self::Match(bitmap) => bitmap.clone(),
            Self::Conjunction(filter_exprs) => {
                // Defensive: resolve never builds an empty compound
                if filter_exprs.is_empty() {
                    return Bitmap::new();
                }

                let mut result = filter_exprs[0].evaluate();
                for expr in filter_exprs.iter().skip(1) {
                    result &= expr.evaluate();
                    if result.is_empty() {
                        break; // already empty - an AND stays empty
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
