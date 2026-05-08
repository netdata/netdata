use siphasher::sip::SipHasher24;
use std::hash::Hasher;

pub fn jenkins_hash64(data: &[u8]) -> u64 {
    use hashers::jenkins::Lookup3Hasher;

    if data.is_empty() {
        // systemd's jenkins_hashlittle2() starts both halves from 0xdeadbeef.
        return 0xdead_beef_dead_beef;
    }

    let mut hasher = Lookup3Hasher::default();
    hasher.write(data);
    let hash = hasher.finish();

    // Jenkins lookup3 returns two 32-bit values (pc, pb)
    // systemd expects: first 32-bit as high part, second 32-bit as low part
    // But Lookup3Hasher::finish() returns them in opposite order, so swap them
    let low = (hash & 0xFFFFFFFF) as u32;
    let high = (hash >> 32) as u32;
    ((low as u64) << 32) | (high as u64)
}

pub fn siphash24(data: &[u8], key: &[u8; 16]) -> u64 {
    let k0 = u64::from_le_bytes(key[0..8].try_into().unwrap());
    let k1 = u64::from_le_bytes(key[8..16].try_into().unwrap());

    let mut hasher = SipHasher24::new_with_keys(k0, k1);
    hasher.write(data);
    hasher.finish()
}

pub fn journal_hash_data(data: &[u8], is_keyed_hash: bool, file_id: Option<&[u8; 16]>) -> u64 {
    if is_keyed_hash {
        if let Some(file_id) = file_id {
            siphash24(data, file_id)
        } else {
            // FIXME: verify fallback behaviour
            jenkins_hash64(data)
        }
    } else {
        jenkins_hash64(data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn jenkins_hash64_matches_systemd_lookup3_values() {
        let cases: &[(&[u8], u64)] = &[
            (b"", 0xdead_beef_dead_beef),
            (b"SYSLOG_IDENTIFIER=netdata", 0x45cc_d0e9_ed13_614a),
            (b"_SYSTEMD_UNIT=netdata.service", 0x1013_c5df_11a9_83f0),
            (b"PRIORITY=6", 0x80f0_9f19_808d_26a3),
            (b"MESSAGE=Test message", 0x8ed5_3fb5_2aa5_c55d),
        ];

        for (payload, expected) in cases {
            assert_eq!(jenkins_hash64(payload), *expected);
        }
    }
}
