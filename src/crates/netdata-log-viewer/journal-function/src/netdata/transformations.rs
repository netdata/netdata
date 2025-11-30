use std::collections::HashMap;
use std::sync::Arc;

/// Trait for field transformations
pub trait FieldTransformation: Send + Sync {
    /// Transform a raw field value to a display representation
    fn transform(&self, raw_value: &str) -> String;
}

/// Registry of field transformations
#[derive(Clone)]
pub struct TransformationRegistry {
    transformations: HashMap<String, Arc<dyn FieldTransformation>>,
}

impl TransformationRegistry {
    /// Create a new empty registry
    pub fn new() -> Self {
        Self {
            transformations: HashMap::new(),
        }
    }

    /// Register a transformation for a specific field
    pub fn register(
        &mut self,
        field_name: impl Into<String>,
        transform: Arc<dyn FieldTransformation>,
    ) {
        self.transformations.insert(field_name.into(), transform);
    }

    /// Transform a field value using the registered transformation
    pub fn transform_field(
        &self,
        field_name: &str,
        raw: Option<String>,
    ) -> journal_engine::CellValue {
        match raw {
            None => journal_engine::CellValue::new(None),
            Some(raw_str) => {
                let display = self
                    .transformations
                    .get(field_name)
                    .map(|t| t.transform(&raw_str))
                    .unwrap_or_else(|| raw_str.clone());

                journal_engine::CellValue::with_display(Some(raw_str), Some(display))
            }
        }
    }

    /// Transform a string value for a field, returning the transformed string.
    ///
    /// If no transformation is registered for the field, returns the original value.
    pub fn transform_value(&self, field_name: &str, value: &str) -> String {
        self.transformations
            .get(field_name)
            .map(|t| t.transform(value))
            .unwrap_or_else(|| value.to_string())
    }
}

impl Default for TransformationRegistry {
    fn default() -> Self {
        Self::new()
    }
}

/// PRIORITY: 0-7 → human-readable names
pub struct PriorityTransformation;

impl FieldTransformation for PriorityTransformation {
    fn transform(&self, raw_value: &str) -> String {
        match raw_value {
            "0" => "panic".to_string(),
            "1" => "alert".to_string(),
            "2" => "critical".to_string(),
            "3" => "error".to_string(),
            "4" => "warning".to_string(),
            "5" => "notice".to_string(),
            "6" => "info".to_string(),
            "7" => "debug".to_string(),
            _ => raw_value.to_string(),
        }
    }
}

/// SYSLOG_FACILITY: 0-23 → facility names
pub struct SyslogFacilityTransformation;

impl FieldTransformation for SyslogFacilityTransformation {
    fn transform(&self, raw_value: &str) -> String {
        match raw_value {
            "0" => "kern".to_string(),
            "1" => "user".to_string(),
            "2" => "mail".to_string(),
            "3" => "daemon".to_string(),
            "4" => "auth".to_string(),
            "5" => "syslog".to_string(),
            "6" => "lpr".to_string(),
            "7" => "news".to_string(),
            "8" => "uucp".to_string(),
            "9" => "cron".to_string(),
            "10" => "authpriv".to_string(),
            "11" => "ftp".to_string(),
            "12" => "ntp".to_string(),
            "13" => "security".to_string(),
            "14" => "console".to_string(),
            "15" => "solaris-cron".to_string(),
            "16" => "local0".to_string(),
            "17" => "local1".to_string(),
            "18" => "local2".to_string(),
            "19" => "local3".to_string(),
            "20" => "local4".to_string(),
            "21" => "local5".to_string(),
            "22" => "local6".to_string(),
            "23" => "local7".to_string(),
            _ => raw_value.to_string(),
        }
    }
}

/// ERRNO: numeric → "code (name)"
pub struct ErrnoTransformation;

impl FieldTransformation for ErrnoTransformation {
    fn transform(&self, raw_value: &str) -> String {
        let name = match raw_value {
            "1" => "EPERM",
            "2" => "ENOENT",
            "3" => "ESRCH",
            "4" => "EINTR",
            "5" => "EIO",
            "6" => "ENXIO",
            "7" => "E2BIG",
            "8" => "ENOEXEC",
            "9" => "EBADF",
            "10" => "ECHILD",
            "11" => "EAGAIN",
            "12" => "ENOMEM",
            "13" => "EACCES",
            "14" => "EFAULT",
            "15" => "ENOTBLK",
            "16" => "EBUSY",
            "17" => "EEXIST",
            "18" => "EXDEV",
            "19" => "ENODEV",
            "20" => "ENOTDIR",
            "21" => "EISDIR",
            "22" => "EINVAL",
            "23" => "ENFILE",
            "24" => "EMFILE",
            "25" => "ENOTTY",
            "26" => "ETXTBSY",
            "27" => "EFBIG",
            "28" => "ENOSPC",
            "29" => "ESPIPE",
            "30" => "EROFS",
            "31" => "EMLINK",
            "32" => "EPIPE",
            "33" => "EDOM",
            "34" => "ERANGE",
            "35" => "EDEADLK",
            "36" => "ENAMETOOLONG",
            "37" => "ENOLCK",
            "38" => "ENOSYS",
            "39" => "ENOTEMPTY",
            "40" => "ELOOP",
            "42" => "ENOMSG",
            "43" => "EIDRM",
            "44" => "ECHRNG",
            "45" => "EL2NSYNC",
            "46" => "EL3HLT",
            "47" => "EL3RST",
            "48" => "ELNRNG",
            "49" => "EUNATCH",
            "50" => "ENOCSI",
            "51" => "EL2HLT",
            "52" => "EBADE",
            "53" => "EBADR",
            "54" => "EXFULL",
            "55" => "ENOANO",
            "56" => "EBADRQC",
            "57" => "EBADSLT",
            "59" => "EBFONT",
            "60" => "ENOSTR",
            "61" => "ENODATA",
            "62" => "ETIME",
            "63" => "ENOSR",
            "64" => "ENONET",
            "65" => "ENOPKG",
            "66" => "EREMOTE",
            "67" => "ENOLINK",
            "68" => "EADV",
            "69" => "ESRMNT",
            "70" => "ECOMM",
            "71" => "EPROTO",
            "72" => "EMULTIHOP",
            "73" => "EDOTDOT",
            "74" => "EBADMSG",
            "75" => "EOVERFLOW",
            "76" => "ENOTUNIQ",
            "77" => "EBADFD",
            "78" => "EREMCHG",
            "79" => "ELIBACC",
            "80" => "ELIBBAD",
            "81" => "ELIBSCN",
            "82" => "ELIBMAX",
            "83" => "ELIBEXEC",
            "84" => "EILSEQ",
            "85" => "ERESTART",
            "86" => "ESTRPIPE",
            "87" => "EUSERS",
            "88" => "ENOTSOCK",
            "89" => "EDESTADDRREQ",
            "90" => "EMSGSIZE",
            "91" => "EPROTOTYPE",
            "92" => "ENOPROTOOPT",
            "93" => "EPROTONOSUPPORT",
            "94" => "ESOCKTNOSUPPORT",
            "95" => "EOPNOTSUPP",
            "96" => "EPFNOSUPPORT",
            "97" => "EAFNOSUPPORT",
            "98" => "EADDRINUSE",
            "99" => "EADDRNOTAVAIL",
            "100" => "ENETDOWN",
            "101" => "ENETUNREACH",
            "102" => "ENETRESET",
            "103" => "ECONNABORTED",
            "104" => "ECONNRESET",
            "105" => "ENOBUFS",
            "106" => "EISCONN",
            "107" => "ENOTCONN",
            "108" => "ESHUTDOWN",
            "109" => "ETOOMANYREFS",
            "110" => "ETIMEDOUT",
            "111" => "ECONNREFUSED",
            "112" => "EHOSTDOWN",
            "113" => "EHOSTUNREACH",
            "114" => "EALREADY",
            "115" => "EINPROGRESS",
            "116" => "ESTALE",
            "117" => "EUCLEAN",
            "118" => "ENOTNAM",
            "119" => "ENAVAIL",
            "120" => "EISNAM",
            "121" => "EREMOTEIO",
            "122" => "EDQUOT",
            "123" => "ENOMEDIUM",
            "124" => "EMEDIUMTYPE",
            "125" => "ECANCELED",
            "126" => "ENOKEY",
            "127" => "EKEYEXPIRED",
            "128" => "EKEYREVOKED",
            "129" => "EKEYREJECTED",
            "130" => "EOWNERDEAD",
            "131" => "ENOTRECOVERABLE",
            "132" => "ERFKILL",
            "133" => "EHWPOISON",
            _ => return raw_value.to_string(),
        };

        name.to_string()
    }
}

/// _BOOT_ID: UUID → "UUID (timestamp)"
/// Note: Full implementation would require boot_id cache lookup
pub struct BootIdTransformation;

impl FieldTransformation for BootIdTransformation {
    fn transform(&self, raw_value: &str) -> String {
        // For now, just return the UUID
        // Full implementation would look up first boot timestamp from cache
        raw_value.to_string()
    }
}

/// _UID: numeric → username
pub struct UidTransformation;

impl FieldTransformation for UidTransformation {
    fn transform(&self, raw_value: &str) -> String {
        use nix::unistd::{Uid, User};

        // Parse the UID
        let Ok(uid_num) = raw_value.parse::<u32>() else {
            return raw_value.to_string();
        };

        let uid = Uid::from_raw(uid_num);

        // Look up the user
        match User::from_uid(uid) {
            Ok(Some(user)) => user.name,
            Ok(None) => raw_value.to_string(), // User not found
            Err(_) => raw_value.to_string(),   // Lookup error
        }
    }
}

/// _GID: numeric → groupname
pub struct GidTransformation;

impl FieldTransformation for GidTransformation {
    fn transform(&self, raw_value: &str) -> String {
        use nix::unistd::{Gid, Group};

        // Parse the GID
        let Ok(gid_num) = raw_value.parse::<u32>() else {
            return raw_value.to_string();
        };

        let gid = Gid::from_raw(gid_num);

        // Look up the group
        match Group::from_gid(gid) {
            Ok(Some(group)) => group.name,
            Ok(None) => raw_value.to_string(), // Group not found
            Err(_) => raw_value.to_string(),   // Lookup error
        }
    }
}

/// _CAP_EFFECTIVE: hex → "hex (capability names)"
pub struct CapEffectiveTransformation;

impl FieldTransformation for CapEffectiveTransformation {
    fn transform(&self, raw_value: &str) -> String {
        // Parse hex value
        let caps_value = if let Some(hex) = raw_value.strip_prefix("0x") {
            u64::from_str_radix(hex, 16).ok()
        } else {
            raw_value.parse::<u64>().ok()
        };

        let Some(caps) = caps_value else {
            return raw_value.to_string();
        };

        // Linux capabilities (41 capabilities as of Linux 5.x)
        const CAPABILITIES: &[&str] = &[
            "CAP_CHOWN",
            "CAP_DAC_OVERRIDE",
            "CAP_DAC_READ_SEARCH",
            "CAP_FOWNER",
            "CAP_FSETID",
            "CAP_KILL",
            "CAP_SETGID",
            "CAP_SETUID",
            "CAP_SETPCAP",
            "CAP_LINUX_IMMUTABLE",
            "CAP_NET_BIND_SERVICE",
            "CAP_NET_BROADCAST",
            "CAP_NET_ADMIN",
            "CAP_NET_RAW",
            "CAP_IPC_LOCK",
            "CAP_IPC_OWNER",
            "CAP_SYS_MODULE",
            "CAP_SYS_RAWIO",
            "CAP_SYS_CHROOT",
            "CAP_SYS_PTRACE",
            "CAP_SYS_PACCT",
            "CAP_SYS_ADMIN",
            "CAP_SYS_BOOT",
            "CAP_SYS_NICE",
            "CAP_SYS_RESOURCE",
            "CAP_SYS_TIME",
            "CAP_SYS_TTY_CONFIG",
            "CAP_MKNOD",
            "CAP_LEASE",
            "CAP_AUDIT_WRITE",
            "CAP_AUDIT_CONTROL",
            "CAP_SETFCAP",
            "CAP_MAC_OVERRIDE",
            "CAP_MAC_ADMIN",
            "CAP_SYSLOG",
            "CAP_WAKE_ALARM",
            "CAP_BLOCK_SUSPEND",
            "CAP_AUDIT_READ",
            "CAP_PERFMON",
            "CAP_BPF",
            "CAP_CHECKPOINT_RESTORE",
        ];

        let mut cap_names = Vec::new();
        for (i, &cap_name) in CAPABILITIES.iter().enumerate() {
            if caps & (1u64 << i) != 0 {
                cap_names.push(cap_name);
            }
        }

        if cap_names.is_empty() {
            "none".to_string()
        } else {
            cap_names.join(", ")
        }
    }
}

/// _SOURCE_REALTIME_TIMESTAMP: microseconds → "microseconds (ISO8601)"
pub struct SourceRealtimeTimestampTransformation;

impl FieldTransformation for SourceRealtimeTimestampTransformation {
    fn transform(&self, raw_value: &str) -> String {
        // Parse microseconds since epoch
        let Ok(usec) = raw_value.parse::<i64>() else {
            return raw_value.to_string();
        };

        // Convert to seconds and nanoseconds
        let secs = usec / 1_000_000;
        let nsecs = ((usec % 1_000_000) * 1000) as u32;

        // Create DateTime in UTC, then convert to local timezone
        use chrono::{Local, TimeZone, Utc};
        let dt_utc = match Utc.timestamp_opt(secs, nsecs) {
            chrono::LocalResult::Single(dt) => dt,
            _ => return raw_value.to_string(),
        };

        // Convert to local time
        let dt_local = dt_utc.with_timezone(&Local);

        // Format as RFC3339 with microsecond precision in local timezone
        dt_local.to_rfc3339_opts(chrono::SecondsFormat::Micros, true)
    }
}

/// MESSAGE_ID: UUID → "UUID (description)"
pub struct MessageIdTransformation;

impl FieldTransformation for MessageIdTransformation {
    fn transform(&self, raw_value: &str) -> String {
        // Known journal message IDs from systemd and other sources
        let description = match raw_value {
            "f77379a8490b408bbe5f6940505a777b" => "Journal started",
            "d93fb3c9c24d451a97cea615ce59c00b" => "Journal stopped",
            "a596d6fe7bfa4994828e72309e95d61e" => "Journal messages suppressed",
            "e9bf28e6e834481bb6f48f548ad13606" => "Journal messages missed",
            "ec387f577b844b8fa948f33cad9a75e6" => "Journal disk space usage",
            "fc2e22bc6ee647b6b90729ab34a250b1" => "Coredump",
            "5aadd8e954dc4b1a8c954d63fd9e1137" => "Coredump truncated",
            "1f4e0a44a88649939aaea34fc6da8c95" => "Backtrace",
            "8d45620c1a4348dbb17410da57c60c66" => "User Session created",
            "3354939424b4456d9802ca8333ed424a" => "User Session terminated",
            "fcbefc5da23d428093f97c82a9290f7b" => "Seat started",
            "e7852bfe46784ed0accde04bc864c2d5" => "Seat removed",
            "24d8d4452573402496068381a6312df2" => "VM or container started",
            "58432bd3bace477cb514b56381b8a758" => "VM or container stopped",
            "c7a787079b354eaaa9e77b371893cd27" => "Time change",
            "45f82f4aef7a4bbf942ce861d1f20990" => "Timezone change",
            "50876a9db00f4c40bde1a2ad381c3a1b" => "System configuration issues",
            "b07a249cd024414a82dd00cd181378ff" => "System start-up completed",
            "eed00a68ffd84e31882105fd973abdd1" => "User start-up completed",
            "6bbd95ee977941e497c48be27c254128" => "Sleep start",
            "8811e6df2a8e40f58a94cea26f8ebf14" => "Sleep stop",
            "98268866d1d54a499c4e98921d93bc40" => "System shutdown initiated",
            "c14aaf76ec284a5fa1f105f88dfb061c" => "System factory reset initiated",
            "d9ec5e95e4b646aaaea2fd05214edbda" => "Container init crashed",
            "3ed0163e868a4417ab8b9e210407a96c" => "System reboot failed after crash",
            "645c735537634ae0a32b15a7c6cba7d4" => "Init execution froze",
            "5addb3a06a734d3396b794bf98fb2d01" => "Init crashed no coredump",
            "5c9e98de4ab94c6a9d04d0ad793bd903" => "Init crashed no fork",
            "5e6f1f5e4db64a0eaee3368249d20b94" => "Init crashed unknown signal",
            "83f84b35ee264f74a3896a9717af34cb" => "Init crashed systemd signal",
            "3a73a98baf5b4b199929e3226c0be783" => "Init crashed process signal",
            "2ed18d4f78ca47f0a9bc25271c26adb4" => "Init crashed waitpid failed",
            "56b1cd96f24246c5b607666fda952356" => "Init crashed coredump failed",
            "4ac7566d4d7548f4981f629a28f0f829" => "Init crashed coredump",
            "38e8b1e039ad469291b18b44c553a5b7" => "Crash shell failed to fork",
            "872729b47dbe473eb768ccecd477beda" => "Crash shell failed to execute",
            "658a67adc1c940b3b3316e7e8628834a" => "Selinux failed",
            "e6f456bd92004d9580160b2207555186" => "Battery low warning",
            "267437d33fdd41099ad76221cc24a335" => "Battery low powering off",
            "79e05b67bc4545d1922fe47107ee60c5" => "Manager mainloop failed",
            "dbb136b10ef4457ba47a795d62f108c9" => "Manager no xdgdir path",
            "ed158c2df8884fa584eead2d902c1032" => {
                "Init failed to drop capability bounding set of usermode"
            }
            "42695b500df048298bee37159caa9f2e" => "Init failed to drop capability bounding set",
            "bfc2430724ab44499735b4f94cca9295" => "User manager can't disable new privileges",
            "59288af523be43a28d494e41e26e4510" => "Manager failed to start default target",
            "689b4fcc97b4486ea5da92db69c9e314" => "Manager failed to isolate default target",
            "5ed836f1766f4a8a9fc5da45aae23b29" => {
                "Manager failed to collect passed file descriptors"
            }
            "6a40fbfbd2ba4b8db02fb40c9cd090d7" => "Init failed to fix up environment variables",
            "0e54470984ac419689743d957a119e2e" => "Manager failed to allocate",
            "d67fa9f847aa4b048a2ae33535331adb" => "Manager failed to write Smack",
            "af55a6f75b544431b72649f36ff6d62c" => "System shutdown critical error",
            "d18e0339efb24a068d9c1060221048c2" => "Init failed to fork off valgrind",
            "7d4958e842da4a758f6c1cdc7b36dcc5" => "Unit starting",
            "39f53479d3a045ac8e11786248231fbf" => "Unit started",
            "be02cf6855d2428ba40df7e9d022f03d" => "Unit failed",
            "de5b426a63be47a7b6ac3eaac82e2f6f" => "Unit stopping",
            "9d1aaa27d60140bd96365438aad20286" => "Unit stopped",
            "d34d037fff1847e6ae669a370e694725" => "Unit reloading",
            "7b05ebc668384222baa8881179cfda54" => "Unit reloaded",
            "5eb03494b6584870a536b337290809b3" => "Unit restart scheduled",
            "ae8f7b866b0347b9af31fe1c80b127c0" => "Unit resources",
            "7ad2d189f7e94e70a38c781354912448" => "Unit success",
            "0e4284a0caca4bfc81c0bb6786972673" => "Unit skipped",
            "d9b373ed55a64feb8242e02dbe79a49c" => "Unit failure result",
            "641257651c1b4ec9a8624d7a40a9e1e7" => "Process execution failed",
            "98e322203f7a4ed290d09fe03c09fe15" => "Unit process exited",
            "0027229ca0644181a76c4e92458afa2e" => "Syslog forward missed",
            "1dee0369c7fc4736b7099b38ecb46ee7" => "Mount point is not empty",
            "d989611b15e44c9dbf31e3c81256e4ed" => "Unit oomd kill",
            "fe6faa94e7774663a0da52717891d8ef" => "Unit out of memory",
            "b72ea4a2881545a0b50e200e55b9b06f" => "Lid opened",
            "b72ea4a2881545a0b50e200e55b9b070" => "Lid closed",
            "f5f416b862074b28927a48c3ba7d51ff" => "System docked",
            "51e171bd585248568110144c517cca53" => "System undocked",
            "b72ea4a2881545a0b50e200e55b9b071" => "Power key",
            "3e0117101eb243c1b9a50db3494ab10b" => "Power key long press",
            "9fa9d2c012134ec385451ffe316f97d0" => "Reboot key",
            "f1c59a58c9d943668965c337caec5975" => "Reboot key long press",
            "b72ea4a2881545a0b50e200e55b9b072" => "Suspend key",
            "bfdaf6d312ab4007bc1fe40a15df78e8" => "Suspend key long press",
            "b72ea4a2881545a0b50e200e55b9b073" => "Hibernate key",
            "167836df6f7f428e98147227b2dc8945" => "Hibernate key long press",
            "c772d24e9a884cbeb9ea12625c306c01" => "Invalid configuration",
            "1675d7f172174098b1108bf8c7dc8f5d" => "DNSSEC validation failed",
            "4d4408cfd0d144859184d1e65d7c8a65" => "DNSSEC trust anchor revoked",
            "36db2dfa5a9045e1bd4af5f93e1cf057" => "DNSSEC turned off",
            "b61fdac612e94b9182285b998843061f" => "Username unsafe",
            "1b3bb94037f04bbf81028e135a12d293" => "Mount point path not suitable",
            "010190138f494e29a0ef6669749531aa" => "Device path not suitable",
            "b480325f9c394a7b802c231e51a2752c" => "Nobody user unsuitable",
            "1c0454c1bd2241e0ac6fefb4bc631433" => "Systemd udev settle deprecated",
            "7c8a41f37b764941a0e1780b1be2f037" => "Time initial sync",
            "7db73c8af0d94eeb822ae04323fe6ab6" => "Time initial bump",
            "9e7066279dc8403da79ce4b1a69064b2" => "Shutdown scheduled",
            "249f6fb9e6e2428c96f3f0875681ffa3" => "Shutdown canceled",
            "3f7d5ef3e54f4302b4f0b143bb270cab" => "TPM PCR Extended",
            "f9b0be465ad540d0850ad32172d57c21" => "Memory Trimmed",
            "a8fa8dacdb1d443e9503b8be367a6adb" => "SysV Service Found",
            "187c62eb1e7f463bb530394f52cb090f" => "Portable Service attached",
            "76c5c754d628490d8ecba4c9d042112b" => "Portable Service detached",
            "9cf56b8baf9546cf9478783a8de42113" => {
                "systemd-networkd sysctl changed by foreign process"
            }
            "ad7089f928ac4f7ea00c07457d47ba8a" => "SRK into TPM authorization failure",
            "b2bcbaf5edf948e093ce50bbea0e81ec" => "Secure Attention Key (SAK) was pressed",
            "7fc63312330b479bb32e598d47cef1a8" => "dbus activate no unit",
            "ee9799dab1e24d81b7bee7759a543e1b" => "dbus activate masked unit",
            "a0fa58cafd6f4f0c8d003d16ccf9e797" => "dbus broker exited",
            "c8c6cde1c488439aba371a664353d9d8" => "dbus dirwatch",
            "8af3357071af4153af414daae07d38e7" => "dbus dispatch stats",
            "199d4300277f495f84ba4028c984214c" => "dbus no sopeergroup",
            "b209c0d9d1764ab38d13b8e00d1784d6" => "dbus protocol violation",
            "6fa70fa776044fa28be7a21daf42a108" => "dbus receive failed",
            "0ce0fa61d1a9433dabd67417f6b8e535" => "dbus service failed open",
            "24dc708d9e6a4226a3efe2033bb744de" => "dbus service invalid",
            "f15d2347662d483ea9bcd8aa1a691d28" => "dbus sighup",
            "0ce153587afa4095832d233c17a88001" => "Gnome SM startup succeeded",
            "10dd2dc188b54a5e98970f56499d1f73" => "Gnome SM unrecoverable failure",
            "f3ea493c22934e26811cd62abe8e203a" => "Gnome shell started",
            "c7b39b1e006b464599465e105b361485" => "Flatpak cache",
            "75ba3deb0af041a9a46272ff85d9e73e" => "Flathub pulls",
            "f02bce89a54e4efab3a94a797d26204a" => "Flathub pull errors",
            "dd11929c788e48bdbb6276fb5f26b08a" => "Boltd starting",
            "1e6061a9fbd44501b3ccc368119f2b69" => "Netdata startup",
            "ed4cdb8f1beb4ad3b57cb3cae2d162fa" => "Netdata connection from child",
            "6e2e3839067648968b646045dbf28d66" => "Netdata connection to parent",
            "9ce0cb58ab8b44df82c4bf1ad9ee22de" => "Netdata alert transition",
            "6db0018e83e34320ae2a659d78019fb7" => "Netdata alert notification",
            "23e93dfccbf64e11aac858b9410d8a82" => "Netdata fatal message",
            "8ddaf5ba33a74078b609250db1e951f3" => "Sensor state transition",
            "ec87a56120d5431bace51e2fb8bba243" => "Netdata log flood protection",
            "acb33cb95778476baac702eb7e4e151d" => "Netdata Cloud connection",
            "d1f59606dd4d41e3b217a0cfcae8e632" => "Netdata extreme cardinality",
            "02f47d350af5449197bf7a95b605a468" => "Netdata exit reason",
            "4fdf40816c124623a032b7fe73beacb8" => "Netdata dynamic configuration",
            _ => return raw_value.to_string(),
        };

        description.to_string()
    }
}

/// Create a transformation registry with all systemd journal transformations
pub fn systemd_transformations() -> TransformationRegistry {
    let mut registry = TransformationRegistry::new();

    // Timestamp transformation (used for the first column)
    registry.register("timestamp", Arc::new(SourceRealtimeTimestampTransformation));

    registry.register("PRIORITY", Arc::new(PriorityTransformation));
    registry.register("SYSLOG_FACILITY", Arc::new(SyslogFacilityTransformation));
    registry.register("ERRNO", Arc::new(ErrnoTransformation));
    registry.register("_BOOT_ID", Arc::new(BootIdTransformation));
    registry.register("_UID", Arc::new(UidTransformation));
    registry.register("_GID", Arc::new(GidTransformation));
    registry.register("_CAP_EFFECTIVE", Arc::new(CapEffectiveTransformation));
    registry.register(
        "_SOURCE_REALTIME_TIMESTAMP",
        Arc::new(SourceRealtimeTimestampTransformation),
    );
    registry.register("MESSAGE_ID", Arc::new(MessageIdTransformation));

    // Also register variations that exist in the wild
    registry.register("OBJECT_UID", Arc::new(UidTransformation));
    registry.register("OBJECT_GID", Arc::new(GidTransformation));
    registry.register("_SYSTEMD_OWNER_UID", Arc::new(UidTransformation));
    registry.register("OBJECT_SYSTEMD_OWNER_UID", Arc::new(UidTransformation));
    registry.register("_AUDIT_LOGINUID", Arc::new(UidTransformation));
    registry.register("OBJECT_AUDIT_LOGINUID", Arc::new(UidTransformation));

    registry
}
