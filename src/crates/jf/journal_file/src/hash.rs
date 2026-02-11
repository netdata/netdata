use siphasher::sip::SipHasher24;
use std::hash::Hasher;

fn jenkins_hash64(data: &[u8]) -> u64 {
    // FIXME: user real jenkins hasher
    let mut hasher = twox_hash::XxHash64::default();
    hasher.write(data);
    hasher.finish()
}

fn siphash24(data: &[u8], key: &[u8; 16]) -> u64 {
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
