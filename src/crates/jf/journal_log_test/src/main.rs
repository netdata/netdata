use journal_log::{JournalLog, JournalLogConfig};
use std::path::Path;
use std::time::Duration;

fn print_directory_info(dir_path: &str, scenario: &str) {
    println!("\n=== Directory Info ({}) ===", scenario);
    if let Ok(entries) = std::fs::read_dir(dir_path) {
        let mut files: Vec<_> = entries
            .filter_map(|e| e.ok())
            .filter(|e| e.path().extension().map_or(false, |ext| ext == "journal"))
            .collect();
        files.sort_by_key(|e| e.file_name());

        println!("Journal files found: {}", files.len());
        for entry in files {
            if let Ok(metadata) = entry.metadata() {
                println!(
                    "  {}: {} bytes",
                    entry.file_name().to_string_lossy(),
                    metadata.len()
                );
            }
        }

        // Show actual disk usage
        if let Ok(output) = std::process::Command::new("du")
            .args(["-h", dir_path])
            .output()
        {
            if let Ok(usage) = String::from_utf8(output.stdout) {
                println!(
                    "Actual disk usage: {}",
                    usage.trim().split_whitespace().next().unwrap_or("unknown")
                );
            }
        }
    }
}

fn clean_test_dir(test_dir: &str) -> Result<(), Box<dyn std::error::Error>> {
    if Path::new(test_dir).exists() {
        std::fs::remove_dir_all(test_dir)?;
    }
    Ok(())
}

fn test_size_based_rotation() -> Result<(), Box<dyn std::error::Error>> {
    let test_dir = "/home/cm/repos/nd/otel-plugin/src/crates/jf/journal_log_dir/";
    clean_test_dir(test_dir)?;

    println!("\n========================================");
    println!("TEST 1: Size-based Rotation");
    println!("========================================");

    let config = JournalLogConfig::new(test_dir);
    // .with_rotation_max_file_size(32 * 1024) // 32KB per file (small for testing)
    // .with_rotation_max_duration(3600) // 1 hour (won't trigger)
    // .with_retention_max_size(1024 * 1024) // 1MB total (generous)
    // .with_retention_max_duration(24 * 3600) // 24 hours
    // .with_retention_max_files(10); // Max 10 files

    println!("Config: 32KB rotation size, 1MB total retention, 10 files max");

    let mut journal = JournalLog::new(config)?;
    print_directory_info(test_dir, "Initial");

    println!("\nWriting entries to trigger size-based rotation...");

    for i in 0..500 {
        let message = format!("ENTRY_{:04}: Size rotation test with substantial content to reach 32KB limit quickly. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation.", i);
        let items_refs: Vec<&[u8]> =
            vec![b"MESSAGE", message.as_bytes(), b"TEST", b"SIZE_ROTATION"];
        journal.write_entry(&items_refs)?;

        if i % 100 == 0 && i > 0 {
            print_directory_info(test_dir, &format!("After {} entries", i));
        }
    }

    print_directory_info(test_dir, "Final Size Test");
    Ok(())
}

fn test_duration_based_rotation() -> Result<(), Box<dyn std::error::Error>> {
    let test_dir = "/home/cm/repos/nd/otel-plugin/src/crates/jf/journal_log_dir/";
    clean_test_dir(test_dir)?;

    println!("\n========================================");
    println!("TEST 2: Duration-based Rotation");
    println!("========================================");

    let config = JournalLogConfig::new(test_dir);
    // .with_rotation_max_file_size(1024 * 1024) // 1MB (won't trigger)
    // .with_rotation_max_duration(3) // 3 seconds duration
    // .with_retention_max_size(10 * 1024 * 1024) // 10MB total
    // .with_retention_max_duration(24 * 3600) // 24 hours
    // .with_retention_max_files(10); // Max 10 files

    println!("Config: 3 second rotation duration, 1MB file size limit, 10MB total retention");

    let mut journal = JournalLog::new(config)?;
    print_directory_info(test_dir, "Initial");

    println!("\nWriting entries with sleep to trigger duration-based rotation...");

    for batch in 0..4 {
        println!("Writing batch {}...", batch + 1);

        // Write multiple entries quickly
        for i in 0..50 {
            let message = format!("BATCH_{}_ENTRY_{:03}: Duration test entry written at different times to test time-based rotation policies.", batch, i);
            let batch_str = format!("{}", batch);
            let items_refs: Vec<&[u8]> = vec![
                b"MESSAGE",
                message.as_bytes(),
                b"TEST",
                b"DURATION_ROTATION",
                b"BATCH",
                batch_str.as_bytes(),
            ];
            journal.write_entry(&items_refs)?;
        }

        print_directory_info(test_dir, &format!("After batch {}", batch + 1));

        if batch < 3 {
            println!("Sleeping 4 seconds to trigger duration rotation...");
            std::thread::sleep(Duration::from_secs(4));
        }
    }

    print_directory_info(test_dir, "Final Duration Test");
    Ok(())
}

fn test_file_count_retention() -> Result<(), Box<dyn std::error::Error>> {
    let test_dir = "/home/cm/repos/nd/otel-plugin/src/crates/jf/journal_log_dir/";
    clean_test_dir(test_dir)?;

    println!("\n========================================");
    println!("TEST 3: File Count Retention");
    println!("========================================");

    let config = JournalLogConfig::new(test_dir);
    // .with_rotation_max_file_size(20 * 1024) // 20KB per file (small)
    // .with_rotation_max_duration(3600) // 1 hour
    // .with_retention_max_size(10 * 1024 * 1024) // 10MB total (generous)
    // .with_retention_max_duration(24 * 3600) // 24 hours
    // .with_retention_max_files(3); // Max 3 files only!

    println!("Config: 20KB rotation, MAX 3 FILES retention, 10MB total, 24h duration");

    let mut journal = JournalLog::new(config)?;
    print_directory_info(test_dir, "Initial");

    println!("\nWriting many entries to create files and test 3-file limit...");

    for i in 0..800 {
        let message = format!("ENTRY_{:04}: File count retention test. This should create many files but only keep the latest 3 due to file count limits. Extra content to reach rotation size faster.", i);
        let items_refs: Vec<&[u8]> = vec![
            b"MESSAGE",
            message.as_bytes(),
            b"TEST",
            b"FILE_COUNT_RETENTION",
        ];
        journal.write_entry(&items_refs)?;

        if i % 150 == 0 && i > 0 {
            print_directory_info(test_dir, &format!("After {} entries", i));
        }
    }

    print_directory_info(test_dir, "Final File Count Test");
    Ok(())
}

fn test_total_size_retention() -> Result<(), Box<dyn std::error::Error>> {
    let test_dir = "/home/cm/repos/nd/otel-plugin/src/crates/jf/journal_log_dir/";
    clean_test_dir(test_dir)?;

    println!("\n========================================");
    println!("TEST 4: Total Size Retention");
    println!("========================================");

    let config = JournalLogConfig::new(test_dir);
    // .with_rotation_max_file_size(25 * 1024) // 25KB per file
    // .with_rotation_max_duration(3600) // 1 hour
    // .with_retention_max_size(80 * 1024) // 80KB total (should allow ~3 files)
    // .with_retention_max_duration(24 * 3600) // 24 hours
    // .with_retention_max_files(20); // Large limit to test size limit

    println!("Config: 25KB rotation, 80KB TOTAL SIZE retention, 20 files max");

    let mut journal = JournalLog::new(config)?;
    print_directory_info(test_dir, "Initial");

    println!("\nWriting entries to exceed total size limit...");

    for i in 0..1200 {
        let message = format!("ENTRY_{:04}: Total size retention test. Should create files until total size reaches 80KB, then start removing oldest files. Extra padding content here to reach size faster.", i);
        let items_refs: Vec<&[u8]> = vec![
            b"MESSAGE",
            message.as_bytes(),
            b"TEST",
            b"TOTAL_SIZE_RETENTION",
        ];
        journal.write_entry(&items_refs)?;

        if i % 200 == 0 && i > 0 {
            print_directory_info(test_dir, &format!("After {} entries", i));
        }
    }

    print_directory_info(test_dir, "Final Total Size Test");
    Ok(())
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("Journal Log Test Suite - Multiple Scenarios");
    println!("===========================================");

    // Uncomment the test you want to run:

    // Test 1: Size-based rotation
    test_size_based_rotation()?;

    // Test 2: Duration-based rotation
    test_duration_based_rotation()?;

    // Test 3: File count retention
    test_file_count_retention()?;

    // Test 4: Total size retention
    test_total_size_retention()?;

    // Test 5: Simple duration rotation
    // test_simple_duration_rotation()?;

    println!("\n===========================================");
    println!("Test completed! Change the commented/uncommented");
    println!("test function calls to run different scenarios.");

    Ok(())
}
