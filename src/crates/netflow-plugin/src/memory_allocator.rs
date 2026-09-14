#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct AllocatorMemorySample {
    pub(crate) heap_in_use_bytes: u64,
    pub(crate) heap_free_bytes: u64,
    pub(crate) heap_arena_bytes: u64,
    pub(crate) mmap_in_use_bytes: u64,
    pub(crate) releasable_bytes: u64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct AllocatorTrimResult {
    pub(crate) before: AllocatorMemorySample,
    pub(crate) after: AllocatorMemorySample,
}

const DEFAULT_TRIM_THRESHOLD_BYTES: u64 = 64 * 1024 * 1024;
const PR_SET_THP_DISABLE: libc::c_int = 41;
const NETFLOW_GLIBC_ARENA_MAX: libc::c_int = 1;

#[cfg(all(target_os = "linux", target_env = "gnu"))]
#[repr(C)]
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
struct Mallinfo2Raw {
    arena: libc::size_t,
    ordblks: libc::size_t,
    smblks: libc::size_t,
    hblks: libc::size_t,
    hblkhd: libc::size_t,
    usmblks: libc::size_t,
    fsmblks: libc::size_t,
    uordblks: libc::size_t,
    fordblks: libc::size_t,
    keepcost: libc::size_t,
}

#[cfg(all(target_os = "linux", target_env = "gnu"))]
type Mallinfo2Fn = unsafe extern "C" fn() -> Mallinfo2Raw;

#[cfg(all(target_os = "linux", target_env = "gnu"))]
fn allocator_memory_from_mallinfo2(raw: Mallinfo2Raw) -> AllocatorMemorySample {
    AllocatorMemorySample {
        heap_in_use_bytes: raw.uordblks as u64,
        heap_free_bytes: raw.fordblks as u64,
        heap_arena_bytes: raw.arena as u64,
        mmap_in_use_bytes: raw.hblkhd as u64,
        releasable_bytes: raw.keepcost as u64,
    }
}

#[cfg(all(target_os = "linux", target_env = "gnu"))]
fn allocator_memory_from_mallinfo(raw: libc::mallinfo) -> AllocatorMemorySample {
    fn non_negative(value: libc::c_int) -> u64 {
        u64::try_from(value).unwrap_or_default()
    }

    AllocatorMemorySample {
        heap_in_use_bytes: non_negative(raw.uordblks),
        heap_free_bytes: non_negative(raw.fordblks),
        heap_arena_bytes: non_negative(raw.arena),
        mmap_in_use_bytes: non_negative(raw.hblkhd),
        releasable_bytes: non_negative(raw.keepcost),
    }
}

#[cfg(all(target_os = "linux", target_env = "gnu"))]
unsafe fn mallinfo2_symbol() -> Option<Mallinfo2Fn> {
    let symbol = unsafe { libc::dlsym(libc::RTLD_DEFAULT, b"mallinfo2\0".as_ptr().cast()) };
    if symbol.is_null() {
        return None;
    }

    Some(unsafe { std::mem::transmute::<*mut libc::c_void, Mallinfo2Fn>(symbol) })
}

pub(crate) fn current_allocator_memory() -> AllocatorMemorySample {
    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    unsafe {
        if let Some(mallinfo2) = mallinfo2_symbol() {
            return allocator_memory_from_mallinfo2(mallinfo2());
        }

        return allocator_memory_from_mallinfo(libc::mallinfo());
    }

    #[cfg(not(all(target_os = "linux", target_env = "gnu")))]
    {
        AllocatorMemorySample::default()
    }
}

pub(crate) fn trim_allocator_if_worthwhile() -> Option<AllocatorTrimResult> {
    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    {
        let before = current_allocator_memory();
        if before.heap_free_bytes < DEFAULT_TRIM_THRESHOLD_BYTES
            || before.heap_free_bytes <= before.heap_in_use_bytes
        {
            return None;
        }

        unsafe {
            libc::malloc_trim(0);
        }

        let after = current_allocator_memory();
        return Some(AllocatorTrimResult { before, after });
    }

    #[cfg(not(all(target_os = "linux", target_env = "gnu")))]
    {
        None
    }
}

pub(crate) fn limit_glibc_arenas_for_process() -> Option<libc::c_int> {
    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    unsafe {
        if libc::mallopt(libc::M_ARENA_MAX, NETFLOW_GLIBC_ARENA_MAX) == 1 {
            return Some(NETFLOW_GLIBC_ARENA_MAX);
        }

        return None;
    }

    #[cfg(not(all(target_os = "linux", target_env = "gnu")))]
    {
        None
    }
}

pub(crate) fn disable_transparent_huge_pages_for_process() -> bool {
    #[cfg(target_os = "linux")]
    unsafe {
        return libc::prctl(PR_SET_THP_DISABLE, 1, 0, 0, 0) == 0;
    }

    #[cfg(not(target_os = "linux"))]
    {
        false
    }
}

#[cfg(all(test, target_os = "linux", target_env = "gnu"))]
mod tests {
    use super::*;

    #[test]
    fn allocator_memory_from_mallinfo2_preserves_full_width_values() {
        let sample = allocator_memory_from_mallinfo2(Mallinfo2Raw {
            arena: 5_000_000_000,
            ordblks: 0,
            smblks: 0,
            hblks: 0,
            hblkhd: 1_500_000_000,
            usmblks: 0,
            fsmblks: 0,
            uordblks: 3_000_000_000,
            fordblks: 2_000_000_000,
            keepcost: 64_000_000,
        });

        assert_eq!(
            sample,
            AllocatorMemorySample {
                heap_in_use_bytes: 3_000_000_000,
                heap_free_bytes: 2_000_000_000,
                heap_arena_bytes: 5_000_000_000,
                mmap_in_use_bytes: 1_500_000_000,
                releasable_bytes: 64_000_000,
            }
        );
    }

    #[test]
    fn allocator_memory_from_mallinfo_clamps_negative_fields_to_zero() {
        let sample = allocator_memory_from_mallinfo(libc::mallinfo {
            arena: 1024,
            ordblks: 0,
            smblks: 0,
            hblks: 0,
            hblkhd: -1,
            usmblks: 0,
            fsmblks: 0,
            uordblks: 512,
            fordblks: -1,
            keepcost: 256,
        });

        assert_eq!(
            sample,
            AllocatorMemorySample {
                heap_in_use_bytes: 512,
                heap_free_bytes: 0,
                heap_arena_bytes: 1024,
                mmap_in_use_bytes: 0,
                releasable_bytes: 256,
            }
        );
    }
}
