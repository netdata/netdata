use super::*;

/// Append u32 values as CSV to a String field.
pub(crate) fn append_u32_csv(target: &mut String, values: &[u32]) {
    if values.is_empty() {
        return;
    }
    let mut buf = itoa::Buffer::new();
    for v in values {
        if !target.is_empty() {
            target.push(',');
        }
        target.push_str(buf.format(*v));
    }
}

/// Append large communities as CSV to a String field.
pub(crate) fn append_large_communities_csv(
    target: &mut String,
    values: &[StaticRoutingLargeCommunity],
) {
    if values.is_empty() {
        return;
    }
    let mut asn = itoa::Buffer::new();
    let mut local_data1 = itoa::Buffer::new();
    let mut local_data2 = itoa::Buffer::new();

    for lc in values {
        if !target.is_empty() {
            target.push(',');
        }

        target.push_str(asn.format(lc.asn));
        target.push(':');
        target.push_str(local_data1.format(lc.local_data1));
        target.push(':');
        target.push_str(local_data2.format(lc.local_data2));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn append_u32_csv_formats_into_empty_target() {
        let mut target = String::new();

        append_u32_csv(&mut target, &[10, 20, 30]);

        assert_eq!(target, "10,20,30");
    }

    #[test]
    fn append_u32_csv_appends_after_existing_csv() {
        let mut target = String::from("5");

        append_u32_csv(&mut target, &[10, 20]);

        assert_eq!(target, "5,10,20");
    }

    #[test]
    fn append_u32_csv_ignores_empty_input() {
        let mut target = String::from("existing");

        append_u32_csv(&mut target, &[]);

        assert_eq!(target, "existing");
    }

    #[test]
    fn append_large_communities_csv_formats_without_replacing_existing_csv() {
        let mut target = String::from("64512:1:1");
        let values = [
            StaticRoutingLargeCommunity {
                asn: 64513,
                local_data1: 10,
                local_data2: 20,
            },
            StaticRoutingLargeCommunity {
                asn: 64514,
                local_data1: 30,
                local_data2: 40,
            },
        ];

        append_large_communities_csv(&mut target, &values);

        assert_eq!(target, "64512:1:1,64513:10:20,64514:30:40");
    }

    #[test]
    fn append_large_communities_csv_ignores_empty_input() {
        let mut target = String::from("existing");

        append_large_communities_csv(&mut target, &[]);

        assert_eq!(target, "existing");
    }
}
