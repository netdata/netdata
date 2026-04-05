use super::types::CompiledTransform;
use super::*;

pub(super) fn compile_transform(expression: &str) -> Result<CompiledTransform> {
    let normalized = if expression.trim().is_empty() {
        ".".to_string()
    } else {
        expression.trim().to_string()
    };

    let tokens = jaq_syn::Lexer::new(&normalized)
        .lex()
        .map_err(|errs| anyhow::anyhow!("failed to lex transform '{}': {:?}", normalized, errs))?;
    let main = jaq_syn::Parser::new(&tokens)
        .parse(|parser| parser.module(|module| module.term()))
        .map_err(|errs| anyhow::anyhow!("failed to parse transform '{}': {:?}", normalized, errs))?
        .conv(&normalized);

    let mut ctx = ParseCtx::new(Vec::new());
    ctx.insert_natives(jaq_core::core());
    ctx.insert_defs(jaq_std::std());
    let filter = ctx.compile(main);
    if !ctx.errs.is_empty() {
        let errors = ctx
            .errs
            .into_iter()
            .map(|err| err.0.to_string())
            .collect::<Vec<_>>()
            .join("; ");
        anyhow::bail!("failed to compile transform '{}': {}", normalized, errors);
    }

    Ok(CompiledTransform {
        expression: normalized,
        filter,
    })
}

pub(super) fn run_transform(payload: Value, transform: &CompiledTransform) -> Result<Vec<Value>> {
    let input = Val::from(payload);
    let inputs = RcIter::new(core::iter::empty());
    let mut output = transform.filter.run((Ctx::new([], &inputs), input));
    let mut rows = Vec::new();
    while let Some(next) = output.next() {
        let value = next.map_err(|err| {
            anyhow::anyhow!(
                "failed to execute transform '{}': {}",
                transform.expression,
                err
            )
        })?;
        rows.push(Value::from(value));
    }
    if rows.is_empty() {
        anyhow::bail!("transform '{}' produced empty result", transform.expression);
    }
    Ok(rows)
}
