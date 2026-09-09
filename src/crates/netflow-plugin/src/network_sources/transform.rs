use super::types::CompiledTransform;
use super::*;
use jaq_core::load::{Arena, File, Loader};
use jaq_core::{Compiler, Ctx, Vars, compile, data, load, unwrap_valr};
use jaq_json::Val as JaqVal;

pub(super) fn compile_transform(expression: &str) -> Result<CompiledTransform> {
    let normalized = if expression.trim().is_empty() {
        ".".to_string()
    } else {
        expression.trim().to_string()
    };

    let filter = compile_jaq_filter(&normalized)?;

    Ok(CompiledTransform {
        expression: normalized,
        filter,
    })
}

pub(super) fn run_transform(payload: Value, transform: &CompiledTransform) -> Result<Vec<Value>> {
    let input: JaqVal =
        serde_json::from_value(payload).context("failed to convert JSON payload to jaq value")?;
    let ctx = Ctx::<data::JustLut<JaqVal>>::new(&transform.filter.lut, Vars::new([]));
    let mut output = transform.filter.id.run((ctx, input)).map(unwrap_valr);
    let mut rows = Vec::new();
    while let Some(next) = output.next() {
        let value = next.map_err(|err| {
            anyhow::anyhow!(
                "failed to execute transform '{}': {}",
                transform.expression,
                err
            )
        })?;
        rows.push(jaq_value_to_json(value).with_context(|| {
            format!(
                "failed to convert transform '{}' result to JSON",
                transform.expression
            )
        })?);
    }
    if rows.is_empty() {
        anyhow::bail!("transform '{}' produced empty result", transform.expression);
    }
    Ok(rows)
}

pub(crate) fn compile_jaq_filter(
    expression: &str,
) -> Result<jaq_core::Filter<jaq_core::data::JustLut<JaqVal>>> {
    let defs = jaq_core::defs()
        .chain(jaq_std::defs())
        .chain(jaq_json::defs());
    let funs = jaq_core::funs()
        .chain(jaq_std::funs())
        .chain(jaq_json::funs());
    let loader = Loader::new(defs);
    let arena = Arena::default();
    let modules = loader
        .load(
            &arena,
            File {
                code: expression,
                path: (),
            },
        )
        .map_err(|errs| {
            anyhow::anyhow!(
                "failed to parse transform '{}': {}",
                expression,
                format_load_errors(&errs)
            )
        })?;

    Compiler::<_, data::JustLut<JaqVal>>::default()
        .with_funs(funs)
        .compile(modules)
        .map_err(|errs| {
            anyhow::anyhow!(
                "failed to compile transform '{}': {}",
                expression,
                format_compile_errors(&errs)
            )
        })
}

fn format_load_errors<P>(errs: &load::Errors<&str, P>) -> String {
    let mut parts = Vec::new();
    for (_file, err) in errs {
        match err {
            load::Error::Io(items) => {
                for (_, msg) in items {
                    parts.push(format!("io error: {msg}"));
                }
            }
            load::Error::Lex(items) => {
                for (expected, found) in items {
                    parts.push(format!(
                        "lex error near \"{}\": expected {:?}",
                        truncate_for_display(found),
                        expected
                    ));
                }
            }
            load::Error::Parse(items) => {
                for (expected, input) in items {
                    parts.push(format!(
                        "parse error near \"{}\": expected {:?}",
                        truncate_for_display(input),
                        expected
                    ));
                }
            }
        }
    }
    if parts.is_empty() {
        "unknown error".to_string()
    } else {
        parts.join("; ")
    }
}

fn format_compile_errors<P>(errs: &compile::Errors<&str, P>) -> String {
    let mut parts = Vec::new();
    for (_file, items) in errs {
        for (sym, undefined) in items {
            parts.push(format!(
                "undefined {} \"{}\"",
                undefined.as_str(),
                truncate_for_display(sym)
            ));
        }
    }
    if parts.is_empty() {
        "unknown error".to_string()
    } else {
        parts.join("; ")
    }
}

fn truncate_for_display(s: &str) -> String {
    const MAX: usize = 60;
    let trimmed = s.trim();
    if trimmed.chars().count() <= MAX {
        trimmed.to_string()
    } else {
        let cut: String = trimmed.chars().take(MAX).collect();
        format!("{cut}...")
    }
}

fn jaq_value_to_json(value: JaqVal) -> Result<Value> {
    serde_json::from_str(&value.to_string()).context("jaq value is not representable as JSON")
}
