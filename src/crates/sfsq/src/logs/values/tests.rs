use super::*;

fn top(n: usize, by: bool) -> AggregateSpec {
    AggregateSpec {
        field: "path".into(),
        by: by.then(|| "app".into()),
        kind: AggregateKind::Top(n),
    }
}

fn quantiles(phis: &[f64]) -> AggregateSpec {
    AggregateSpec {
        field: "duration_ms".into(),
        by: None,
        kind: AggregateKind::Quantiles(phis.to_vec()),
    }
}

fn top_values(result: &AggregateResult, group: Option<&str>) -> Vec<(String, u64)> {
    let g = result
        .groups
        .iter()
        .find(|g| g.group.as_deref() == group)
        .expect("group present");
    match &g.answer {
        GroupAnswer::Top { values, .. } => values.clone(),
        other => panic!("not a top answer: {other:?}"),
    }
}

fn quantile_values(result: &AggregateResult) -> Vec<Option<f64>> {
    match &result.groups[0].answer {
        GroupAnswer::Quantiles(q) => q.iter().map(|&(_, v)| v).collect(),
        other => panic!("not a quantiles answer: {other:?}"),
    }
}

#[test]
fn top_orders_by_count_then_value_and_reports_distinct() {
    let spec = top(2, false);
    let mut agg = ValueAgg::new(&spec);
    agg.add_group(None, [("/b", 5), ("/a", 5), ("/c", 9), ("/d", 1)]);
    let result = agg.finish(&spec);
    assert_eq!(
        top_values(&result, None),
        vec![("/c".to_string(), 9), ("/a".to_string(), 5)]
    );
    match &result.groups[0].answer {
        GroupAnswer::Top { distinct, .. } => assert_eq!(*distinct, 4),
        other => panic!("{other:?}"),
    }
    assert_eq!(result.groups[0].count, 20);
    assert!(!result.incomplete && !result.approximate);
}

#[test]
fn merging_split_inputs_equals_one_input() {
    let spec = top(10, true);
    let rows = [
        (Some("a"), "/x", 3),
        (Some("a"), "/y", 1),
        (Some("b"), "/x", 2),
        (None, "/z", 4),
        (Some("a"), "/x", 2),
    ];
    let mut whole = ValueAgg::new(&spec);
    for &(g, v, c) in &rows {
        whole.add(g, v, c);
    }
    let mut left = ValueAgg::new(&spec);
    let mut right = ValueAgg::new(&spec);
    for (i, &(g, v, c)) in rows.iter().enumerate() {
        if i % 2 == 0 {
            left.add(g, v, c)
        } else {
            right.add(g, v, c)
        }
    }
    left.merge(right);
    assert_eq!(left.finish(&spec), whole.finish(&spec));
}

#[test]
fn groups_order_by_count_and_keep_logs_without_group() {
    let spec = top(1, true);
    let mut agg = ValueAgg::new(&spec);
    agg.add(Some("small"), "/x", 1);
    agg.add(Some("big"), "/x", 10);
    agg.add(None, "/y", 5);
    let result = agg.finish(&spec);
    let order: Vec<Option<&str>> = result.groups.iter().map(|g| g.group.as_deref()).collect();
    assert_eq!(order, vec![Some("big"), None, Some("small")]);
}

#[test]
fn exact_quantiles_use_nearest_rank() {
    let spec = quantiles(&[0.0, 0.5, 0.99, 1.0]);
    let mut agg = ValueAgg::new(&spec);
    let values: Vec<String> = (1..=100).map(|v| v.to_string()).collect();
    agg.add_group(None, values.iter().map(|v| (v.as_str(), 1)));
    let result = agg.finish(&spec);
    assert_eq!(
        quantile_values(&result),
        vec![Some(1.0), Some(50.0), Some(99.0), Some(100.0)]
    );
    assert!(!result.approximate);
    assert_eq!(result.groups[0].count, 100);
}

#[test]
fn quantiles_weight_counts_and_ignore_non_numbers() {
    let spec = quantiles(&[0.5, 0.9]);
    let mut agg = ValueAgg::new(&spec);
    agg.add_group(
        None,
        [("10", 8), ("-2.5", 1), ("1e3", 1), ("n/a", 7), ("inf", 1)],
    );
    let result = agg.finish(&spec);
    assert_eq!(quantile_values(&result), vec![Some(10.0), Some(10.0)]);
    assert_eq!(result.groups[0].count, 10);
    assert_eq!(result.ignored, 8);
}

#[test]
fn top_past_the_cap_keeps_the_most_frequent_and_flags_incomplete() {
    let spec = top(3, false);
    let mut agg = ValueAgg::new(&spec);
    let names: Vec<String> = (0..MAX_EXACT_ENTRIES + 10)
        .map(|i| format!("/{i}"))
        .collect();
    agg.add_group(None, names.iter().map(|n| (n.as_str(), 1)));
    agg.add_group(None, [("/hot", 100), ("/warm", 50)]);
    agg.enforce_cap();
    assert!(agg.entries() <= MAX_EXACT_ENTRIES);
    let result = agg.finish(&spec);
    assert!(result.incomplete);
    let top = top_values(&result, None);
    assert_eq!(top[0], ("/hot".to_string(), 100));
    assert_eq!(top[1], ("/warm".to_string(), 50));
}

#[test]
fn quantiles_past_the_cap_switch_to_the_histogram() {
    let spec = quantiles(&[0.5, 0.99]);
    let n = MAX_EXACT_ENTRIES + 1000;
    let mut a = ValueAgg::new(&spec);
    let mut b = ValueAgg::new(&spec);
    // Distinct values split across two shards: each is under the cap, the
    // merge is over it.
    let values: Vec<String> = (0..n).map(|v| format!("{}.5", v)).collect();
    a.add_group(None, values[..n / 2].iter().map(|v| (v.as_str(), 1)));
    b.add_group(None, values[n / 2..].iter().map(|v| (v.as_str(), 1)));
    a.merge(b);
    let result = a.finish(&spec);
    assert!(result.approximate);
    assert_eq!(result.groups[0].count, n as u64);
    let exact = [(n / 2) as f64 - 0.5, (n as f64 * 0.99).ceil() - 0.5];
    for (got, want) in quantile_values(&result).into_iter().zip(exact) {
        let got = got.expect("numeric values recorded");
        assert!((got - want).abs() <= want / 32.0 + 0.001, "{got} vs {want}");
    }
}

#[test]
fn approximate_quantiles_order_negative_values_first() {
    let spec = quantiles(&[0.1, 0.9]);
    let mut agg = ValueAgg::new(&spec);
    agg.add_group(None, [("-100", 5), ("100", 5)]);
    // Force the approximate mode on a small input.
    if let Groups::Quantiles(groups) = &mut agg.groups {
        for numbers in groups.values_mut() {
            numbers.make_approx();
        }
    }
    let result = agg.finish(&spec);
    assert!(result.approximate);
    let q = quantile_values(&result);
    assert!(q[0].unwrap() < 0.0 && (q[0].unwrap() + 100.0).abs() <= 100.0 / 32.0);
    assert!(q[1].unwrap() > 0.0 && (q[1].unwrap() - 100.0).abs() <= 100.0 / 32.0);
}

#[test]
fn empty_aggregation_answers_no_groups() {
    let spec = quantiles(&[0.99]);
    let result = ValueAgg::new(&spec).finish(&spec);
    assert!(result.groups.is_empty());
    assert_eq!(result.ignored, 0);
}

#[test]
fn nearest_rank_survives_float_rounding() {
    // 0.07 * 100 = 7.000000000000001 in f64; the rank is still 7.
    let spec = quantiles(&[0.07, 0.14, 0.29]);
    let mut agg = ValueAgg::new(&spec);
    let values: Vec<String> = (1..=100).map(|v| v.to_string()).collect();
    agg.add_group(None, values.iter().map(|v| (v.as_str(), 1)));
    assert_eq!(
        quantile_values(&agg.finish(&spec)),
        vec![Some(7.0), Some(14.0), Some(29.0)]
    );
}
