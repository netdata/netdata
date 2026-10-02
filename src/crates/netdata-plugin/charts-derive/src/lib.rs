//! The `NetdataChart` derive macro: `netdata-plugin-charts-derive` is a
//! proc-macro crate whose only export is the `#[derive(NetdataChart)]` macro
//! below. It carries no runtime code, and its only direct Cargo dependent is
//! `rt` (`src/crates/netdata-plugin/rt`), which re-exports the macro at its
//! crate root so downstream chart structs (netflow-plugin) never depend on
//! this crate directly.
//!
//! On a named-field struct the derive generates:
//!
//! ```ignore
//! impl rt::charts::ChartDimensions for <Struct> {
//!     fn write_dimensions(&self, __writer: &mut rt::charts::ChartWriter) {
//!         __writer.write_dimension("<field name>", self.<field> as i64);
//!         // ... one call per non-instance field, in declaration order
//!     }
//! }
//! ```
//!
//! The field name is the dimension id verbatim, the value is cast with
//! `as i64`, and fields annotated `x-chart-instance` — the only annotation
//! the macro reads, matched as a substring of a field attribute's rendered
//! tokens — are the instance fields and are never written. The rest of the
//! `x-chart-*` / `x-dimension-*` vocabulary is schemars metadata the macro
//! ignores; rt's `NetdataChart::chart_metadata()` reads it from the generated
//! JSON schema instead (rt/src/charts/chart_trait.rs). Two known consequences:
//!
//! - `x-dimension-hidden` does not drop the SET: a hidden field still emits a
//!   `write_dimension`, while the extractor gives it no DIMENSION line (see
//!   the `ChartDimensions` docs in chart_trait.rs).
//! - The emitted impl hardcodes the `rt::charts::...` paths, so the deriving
//!   crate must depend on the `rt` crate under that unaliased name.
//!
//! rt re-exports the `NetdataChart` trait and this derive macro under one
//! name (derive macros and traits live in different namespaces) next to a
//! blanket impl that turns any `JsonSchema + ChartDimensions` type into a
//! `NetdataChart`; derive `schemars::JsonSchema` alongside this macro and
//! import both from `rt`. The macro declares no helper attributes, so the
//! schemars annotations stay on the type for `chart_metadata()` to consume.

use proc_macro::TokenStream;
use quote::quote;
use syn::{Data, DeriveInput, Fields, parse_macro_input};

/// Derive macro for `NetdataChart`: generates the `rt::charts::ChartDimensions`
/// impl and writes dimension values straight to the `rt::charts::ChartWriter`
/// with no JSON serialization. On the rt side the blanket impl over
/// `JsonSchema + ChartDimensions` also provides the `NetdataChart` trait, and
/// the trait and this derive macro are re-exported under the same name.
///
/// # Input
///
/// A struct with named fields; any other shape is a compile error spanned
/// over the input. Type and lifetime parameters are not carried into the
/// impl, so a generic struct does not compile with this derive.
///
/// Field selection reads exactly one annotation: a field is the instance
/// field when any of its attributes' rendered token text contains the
/// substring `x-chart-instance` (typically schemars
/// `extend("x-chart-instance" = true)`). The attribute value is not parsed,
/// so a `= false` occurrence marks the field too, as does a doc comment that
/// mentions the marker (a doc comment is a `#[doc = ...]` attribute). The
/// instance field is never written. No other annotation is consumed: the
/// field name is the dimension id verbatim, so `x-dimension-name` renames
/// nothing and `x-dimension-hidden` does not drop the SET — the SET then
/// names a dimension with no DIMENSION line (see the `ChartDimensions` docs
/// in rt's chart_trait.rs).
///
/// # Output
///
/// Inside `impl rt::charts::ChartDimensions for <Struct>`, one
/// `__writer.write_dimension("<field name>", self.<field> as i64);` per
/// non-instance field, in declaration order. Field types must be castable to
/// `i64`; the chart sampler places the SETs between the chart's BEGIN and
/// END (see rt/src/charts/writer.rs).
///
/// # Example
///
/// ```ignore
/// #[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize)]
/// #[schemars(
///     extend("x-chart-id" = "cpu.usage"),
///     extend("x-chart-title" = "CPU Usage"),
/// )]
/// struct CpuMetrics {
///     user: u64,
///     system: u64,
///     idle: u64,
/// }
/// ```
///
/// The macro skips fields marked with `x-chart-instance`:
///
/// ```ignore
/// struct CpuCoreMetrics {
///     #[schemars(extend("x-chart-instance" = true))]
///     core_id: String,  // Skipped in dimension output
///     user: u64,
///     system: u64,
/// }
/// ```
#[proc_macro_derive(NetdataChart)]
pub fn derive_netdata_chart(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    let name = &input.ident;

    // Only structs with named fields are accepted; any other shape is a
    // compile error spanned over the whole input.
    let fields = match &input.data {
        Data::Struct(data) => match &data.fields {
            Fields::Named(fields) => &fields.named,
            _ => {
                return syn::Error::new_spanned(
                    &input,
                    "NetdataChart can only be derived for structs with named fields",
                )
                .to_compile_error()
                .into();
            }
        },
        _ => {
            return syn::Error::new_spanned(&input, "NetdataChart can only be derived for structs")
                .to_compile_error()
                .into();
        }
    };

    // One write_dimension call per non-instance field, in declaration order.
    let dimension_writes = fields.iter().filter_map(|field| {
        let field_name = field.ident.as_ref()?;
        let field_name_str = field_name.to_string();

        // The instance marker is a substring match over each field
        // attribute's rendered tokens; the attribute value is not parsed.
        let is_instance_field = field.attrs.iter().any(|attr| {
            // Render the attribute's tokens as text and substring-match.
            let attr_str = quote!(#attr).to_string();
            attr_str.contains("x-chart-instance")
        });

        // Instance fields are never written.
        if is_instance_field {
            return None;
        }

        // Field name verbatim as the dimension id; value cast to i64.
        Some(quote! {
            __writer.write_dimension(#field_name_str, self.#field_name as i64);
        })
    });

    let expanded = quote! {
        impl rt::charts::ChartDimensions for #name {
            fn write_dimensions(&self, __writer: &mut rt::charts::ChartWriter) {
                #(#dimension_writes)*
            }
        }
    };

    TokenStream::from(expanded)
}
