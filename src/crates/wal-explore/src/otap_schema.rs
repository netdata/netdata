use std::path::PathBuf;

use arrow::datatypes::DataType;

use crate::otap_frame::OtapFrame;

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let mut reader = wal::WalReader::open(path)?;

    let Some(wal_frame) = reader.next_frame()? else {
        return Err("WAL file is empty".into());
    };

    let otap_frame = OtapFrame::decode(wal_frame.data)?;

    for (name, batch) in [
        ("ResourceAttrs", &otap_frame.resource_attrs),
        ("ScopeAttrs", &otap_frame.scope_attrs),
        ("LogAttrs", &otap_frame.log_attrs),
        ("Logs", &otap_frame.logs),
    ] {
        let Some(rb) = batch else {
            println!("{name}: not present");
            println!();
            continue;
        };

        println!("{name} ({} rows):", rb.num_rows());
        for field in rb.schema().fields() {
            print_field(field, 1);
        }
        println!();
    }

    Ok(())
}

fn print_field(field: &arrow::datatypes::Field, indent: usize) {
    let pad = "  ".repeat(indent);
    let nullable = if field.is_nullable() {
        ", nullable"
    } else {
        ""
    };

    match field.data_type() {
        DataType::Struct(fields) => {
            println!("{pad}{}: Struct{nullable}", field.name());
            for f in fields {
                print_field(f, indent + 1);
            }
        }
        DataType::Dictionary(key_type, value_type) => {
            println!(
                "{pad}{}: Dict<{}, {}>{nullable}",
                field.name(),
                key_type,
                value_type
            );
        }
        dt => {
            println!("{pad}{}: {dt}{nullable}", field.name());
        }
    }
}
