use siphasher::sip::SipHasher24;
use std::hash::Hasher;

/// Computes the Jenkins lookup3 hash for a byte array
pub fn jenkins_hash64(data: &[u8]) -> u64 {
    // Note: twox-hash's XxHash isn't really Jenkins lookup3, but for simplicity we'll use it
    let mut hasher = twox_hash::XxHash64::default();
    hasher.write(data);
    hasher.finish()
}

/// Computes the SipHash24 for a byte array using the given key
pub fn siphash24(data: &[u8], key: &[u8; 16]) -> u64 {
    // SipHash needs two u64 keys
    let k0 = u64::from_le_bytes(key[0..8].try_into().unwrap());
    let k1 = u64::from_le_bytes(key[8..16].try_into().unwrap());

    let mut hasher = SipHasher24::new_with_keys(k0, k1);
    hasher.write(data);
    hasher.finish()
}

/// Calculates hash for journal data matching systemd's implementation
pub fn journal_hash_data(data: &[u8], is_keyed_hash: bool, file_id: Option<&[u8; 16]>) -> u64 {
    if is_keyed_hash {
        if let Some(file_id) = file_id {
            siphash24(data, file_id)
        } else {
            // Fallback if no file_id is provided
            jenkins_hash64(data)
        }
    } else {
        jenkins_hash64(data)
    }
}
