use crate::enrichment::ValueExpr;

#[derive(Debug, Clone)]
pub(crate) enum ConditionExpr {
    Literal(bool),
    Equals(ValueExpr, ValueExpr),
    NotEquals(ValueExpr, ValueExpr),
    Greater(ValueExpr, ValueExpr),
    GreaterOrEqual(ValueExpr, ValueExpr),
    Less(ValueExpr, ValueExpr),
    LessOrEqual(ValueExpr, ValueExpr),
    In(ValueExpr, ValueExpr),
    Contains(ValueExpr, ValueExpr),
    StartsWith(ValueExpr, ValueExpr),
    EndsWith(ValueExpr, ValueExpr),
    Matches(ValueExpr, ValueExpr),
}
