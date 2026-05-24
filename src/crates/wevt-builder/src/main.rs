// SPDX-License-Identifier: GPL-3.0-or-later
//
// wevt-builder: orchestrate the Microsoft Windows Event Log /
// Event Tracing for Windows resource-compilation pipeline.
//
// Input  (already produced upstream by wevt_netdata_mc_generate):
//   <build-dir>/wevt_netdata.mc
//   <build-dir>/wevt_netdata_manifest.xml
//
// Output:
//   <build-dir>/wevt_netdata.h               (consumed by libnetdata at compile time)
//   <build-dir>/wevt_netdata_manifest.h      (consumed by the ETW provider code)
//   <build-dir>/wevt_netdata.dll             (resource-only DLL, installed alongside the agent)
//
// Pipeline:
//   1. mc.exe -v -b -U <mc_file> <manifest_file>
//      -> emits wevt_netdata.rc, wevt_netdata.h, wevt_netdata_manifest.h
//   2. copy wevt_netdata_manifest.xml -> wevt_netdata_manifest.man
//      (rc.exe wants a .man extension for manifest resources)
//   3. append `1 2004 "wevt_netdata_manifest.man"` to wevt_netdata.rc
//      (resource id 2004 = MUI_MANIFEST_RESOURCE_ID)
//   4. rc.exe /v /fo wevt_netdata.res wevt_netdata.rc
//      -> emits wevt_netdata.res
//   5. link.exe /dll /noentry /machine:x64 /out:wevt_netdata.dll wevt_netdata.res
//      -> emits wevt_netdata.dll
//
// mc.exe and rc.exe ship with the Windows 10 SDK; link.exe ships with the
// Visual Studio C++ toolchain (Hostx64\x64). Both are discovered by
// filesystem scan -- no registry / vswhere dependency, no hardcoded
// version. The latest installed SDK / MSVC version wins.

use std::cmp::Ordering;
use std::env;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::{Command, ExitCode};

const SDK_BASE: &str = r"C:\Program Files (x86)\Windows Kits\10\bin";
const VS_BASES: &[&str] = &[
    r"C:\Program Files\Microsoft Visual Studio\2022",
    r"C:\Program Files (x86)\Microsoft Visual Studio\2022",
    r"C:\Program Files\Microsoft Visual Studio\2019",
];
const VS_EDITIONS: &[&str] = &["Enterprise", "Professional", "Community", "BuildTools"];

fn main() -> ExitCode {
    let args: Vec<String> = env::args().collect();
    if args.len() != 2 {
        eprintln!("usage: {} <build-dir>", args.first().map(String::as_str).unwrap_or("wevt-builder"));
        return ExitCode::from(2);
    }
    let build_dir = PathBuf::from(&args[1]);

    match run(&build_dir) {
        Ok(()) => ExitCode::SUCCESS,
        Err(msg) => {
            eprintln!("wevt-builder: {msg}");
            ExitCode::from(1)
        }
    }
}

fn run(build_dir: &Path) -> Result<(), String> {
    if !build_dir.is_dir() {
        return Err(format!("build directory does not exist: {}", build_dir.display()));
    }

    let mc_file = build_dir.join("wevt_netdata.mc");
    let manifest_file = build_dir.join("wevt_netdata_manifest.xml");
    if !mc_file.is_file() {
        return Err(format!("{} not found (was wevt_netdata_mc_generate run first?)", mc_file.display()));
    }
    if !manifest_file.is_file() {
        return Err(format!("{} not found", manifest_file.display()));
    }

    let sdk_bin = find_sdk_bin()?;
    let vs_bin = find_vs_bin()?;

    let mc = sdk_bin.join("mc.exe");
    let rc = sdk_bin.join("rc.exe");
    let link = vs_bin.join("link.exe");
    for tool in [&mc, &rc, &link] {
        if !tool.is_file() {
            return Err(format!("required tool missing: {}", tool.display()));
        }
    }

    println!("wevt-builder: SDK bin   = {}", sdk_bin.display());
    println!("wevt-builder: VS bin    = {}", vs_bin.display());
    println!("wevt-builder: build dir = {}", build_dir.display());

    // 1. mc.exe -v -b -U <mc> <manifest>
    run_tool(&mc, build_dir, &["-v", "-b", "-U"], &[&mc_file, &manifest_file], "mc.exe")?;

    let rc_file = build_dir.join("wevt_netdata.rc");
    if !rc_file.is_file() {
        return Err(format!("mc.exe did not produce {}", rc_file.display()));
    }

    // 2. copy manifest.xml -> manifest.man (rc.exe expects .man for manifest resources)
    let manifest_man = build_dir.join("wevt_netdata_manifest.man");
    fs::copy(&manifest_file, &manifest_man).map_err(|e| {
        format!("copy {} -> {}: {e}", manifest_file.display(), manifest_man.display())
    })?;

    // 3. append `1 2004 "wevt_netdata_manifest.man"` to the .rc
    {
        let mut f = fs::OpenOptions::new()
            .append(true)
            .open(&rc_file)
            .map_err(|e| format!("open {} for append: {e}", rc_file.display()))?;
        writeln!(f, "1 2004 \"wevt_netdata_manifest.man\"")
            .map_err(|e| format!("write {}: {e}", rc_file.display()))?;
    }

    // 4. rc.exe /v /fo wevt_netdata.res wevt_netdata.rc
    let res_file = build_dir.join("wevt_netdata.res");
    run_tool(
        &rc,
        build_dir,
        &["/v", "/fo"],
        &[&res_file, &rc_file],
        "rc.exe",
    )?;
    if !res_file.is_file() {
        return Err(format!("rc.exe did not produce {}", res_file.display()));
    }

    // 5. link.exe /dll /noentry /machine:x64 /out:<dll> <res>
    let dll_file = build_dir.join("wevt_netdata.dll");
    let out_arg = format!("/out:{}", dll_file.display());
    let status = Command::new(&link)
        .args(["/dll", "/noentry", "/machine:x64"])
        .arg(&out_arg)
        .arg(&res_file)
        .current_dir(build_dir)
        .status()
        .map_err(|e| format!("spawn link.exe: {e}"))?;
    if !status.success() {
        return Err(format!("link.exe exited with status {:?}", status.code()));
    }
    if !dll_file.is_file() {
        return Err(format!("link.exe did not produce {}", dll_file.display()));
    }

    println!(
        "wevt-builder: produced {} and {}",
        build_dir.join("wevt_netdata.h").display(),
        dll_file.display()
    );
    Ok(())
}

fn run_tool(
    tool: &Path,
    cwd: &Path,
    flags: &[&str],
    paths: &[&Path],
    name: &str,
) -> Result<(), String> {
    println!("wevt-builder: running {name}");
    let mut cmd = Command::new(tool);
    cmd.args(flags);
    for p in paths {
        cmd.arg(p);
    }
    cmd.current_dir(cwd);
    let status = cmd
        .status()
        .map_err(|e| format!("spawn {name}: {e}"))?;
    if !status.success() {
        return Err(format!("{name} exited with status {:?}", status.code()));
    }
    Ok(())
}

fn find_sdk_bin() -> Result<PathBuf, String> {
    let base = Path::new(SDK_BASE);
    if !base.is_dir() {
        return Err(format!(
            "Windows SDK base directory not found: {} (install the Windows 10/11 SDK)",
            base.display()
        ));
    }
    let mut versions: Vec<PathBuf> = fs::read_dir(base)
        .map_err(|e| format!("read {}: {e}", base.display()))?
        .filter_map(Result::ok)
        .map(|e| e.path())
        .filter(|p| p.is_dir())
        .filter(|p| {
            p.file_name()
                .and_then(|s| s.to_str())
                .map(|s| s.starts_with("10."))
                .unwrap_or(false)
        })
        .collect();
    if versions.is_empty() {
        return Err(format!("no SDK versions (10.*) found under {}", base.display()));
    }
    // Newest first.
    versions.sort_by(|a, b| version_cmp(basename(b), basename(a)));
    for v in versions {
        let x64 = v.join("x64");
        if x64.join("mc.exe").is_file() && x64.join("rc.exe").is_file() {
            return Ok(x64);
        }
    }
    Err(format!(
        "no SDK x64 directory under {} has both mc.exe and rc.exe",
        base.display()
    ))
}

fn find_vs_bin() -> Result<PathBuf, String> {
    let mut tried = Vec::new();
    for base in VS_BASES {
        let base = Path::new(base);
        if !base.is_dir() {
            continue;
        }
        for edition in VS_EDITIONS {
            let msvc = base.join(edition).join("VC").join("Tools").join("MSVC");
            tried.push(msvc.display().to_string());
            if !msvc.is_dir() {
                continue;
            }
            let mut versions: Vec<PathBuf> = fs::read_dir(&msvc)
                .ok()
                .into_iter()
                .flatten()
                .filter_map(Result::ok)
                .map(|e| e.path())
                .filter(|p| p.is_dir())
                .collect();
            if versions.is_empty() {
                continue;
            }
            versions.sort_by(|a, b| version_cmp(basename(b), basename(a)));
            for v in versions {
                let bin = v.join("bin").join("Hostx64").join("x64");
                if bin.join("link.exe").is_file() {
                    return Ok(bin);
                }
            }
        }
    }
    Err(format!(
        "Visual Studio link.exe not found. Searched: {}",
        tried.join(", ")
    ))
}

fn basename(p: &Path) -> &str {
    p.file_name().and_then(|s| s.to_str()).unwrap_or("")
}

// Compare dotted numeric version strings like "10.0.26100.0" component-wise.
fn version_cmp(a: &str, b: &str) -> Ordering {
    let split = |s: &str| -> Vec<u64> {
        s.split('.').filter_map(|c| c.parse().ok()).collect()
    };
    split(a).cmp(&split(b))
}
