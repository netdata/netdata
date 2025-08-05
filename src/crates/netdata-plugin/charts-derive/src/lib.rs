//! Derive macro for NetdataChart trait
//!
//! This macro generates efficient code for writing chart dimensions directly
//! to the ChartWriter without JSON serialization overhead.

use proc_macro::TokenStream;
use quote::quote;
use syn::{parse_macro_input, Data, DeriveInput, Fields};

/// Derive macro for NetdataChart trait
///
/// Generates a `write_dimensions` method that directly writes dimension values
/// to the ChartWriter without JSON serialization.
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

    // Extract fields from the struct
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

    // Generate write_dimension calls for each field
    let dimension_writes = fields.iter().filter_map(|field| {
        let field_name = field.ident.as_ref()?;
        let field_name_str = field_name.to_string();

        // Check if this field is marked as the instance field by looking for x-chart-instance in any attribute
        let is_instance_field = field.attrs.iter().any(|attr| {
            // Convert attribute to string and check if it contains x-chart-instance
            let attr_str = quote!(#attr).to_string();
            attr_str.contains("x-chart-instance")
        });

        // Skip instance fields
        if is_instance_field {
            return None;
        }

        // Generate the write call
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
