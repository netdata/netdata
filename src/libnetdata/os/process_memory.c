// SPDX-License-Identifier: GPL-3.0-or-later

#include "process_memory.h"
#include "libnetdata/libnetdata.h"

static OS_PROCESS_MEMORY last_process_memory_info = OS_PROCESS_MEMORY_EMPTY;

#if defined(OS_LINUX)
/**
 * Get process memory information for Linux
 *
 * Uses /proc/<pid>/statm and /proc/<pid>/status to collect memory information.
 */
OS_PROCESS_MEMORY os_process_memory(pid_t pid) {
    OS_PROCESS_MEMORY proc_mem = OS_PROCESS_MEMORY_EMPTY;
    char filename[FILENAME_MAX + 1];
    char buffer[4096 + 1];
    long page_size = sysconf(_SC_PAGESIZE);
    
    if (pid == 0)
        pid = getpid();

    // Get statm information (for RSS, total size, shared, text, data)
    size_t len = 0;
    len = strcatz(filename, len, "/proc/", sizeof(filename));
    len += print_uint64(&filename[len], pid);
    len = strcatz(filename, len, "/statm", sizeof(filename));
    if (read_txt_file(filename, buffer, sizeof(buffer)) == 0) {
        unsigned long size = 0, resident = 0, shared = 0, text = 0, lib = 0, data = 0;
        char *pos = buffer;
        size = str2ull(pos, &pos);
        resident = str2ull(pos, &pos);
        shared = str2ull(pos, &pos);
        text = str2ull(pos, &pos);
        lib = str2ull(pos, &pos);
        data = str2ull(pos, &pos);
        (void)lib;
        (void)data;

        proc_mem.virtual_size = size * page_size;
        proc_mem.rss = resident * page_size;
        proc_mem.shared = shared * page_size;
        proc_mem.text = text * page_size;
        proc_mem.data = data * page_size;
    }
    
    // Get status information (for peak resident set size)
    len = 0;
    len = strcatz(filename, len, "/proc/", sizeof(filename));
    len += print_uint64(&filename[len], pid);
    len = strcatz(filename, len, "/status", sizeof(filename));
    if (read_txt_file(filename, buffer, sizeof(buffer)) == 0) {
        char *s = strstr(buffer, "VmHWM:");
        if (s) {
            s += 6; // Skip "VmHWM:"
            while (isspace((uint8_t)*s)) s++; // Skip spaces
            proc_mem.max_rss = str2ull(s, NULL) * 1024; // VmHWM is in kB, convert to bytes
        }
    }
    
    if (OS_PROCESS_MEMORY_OK(proc_mem))
        last_process_memory_info = proc_mem;
        
    return proc_mem;
}
#endif

#if defined(OS_FREEBSD)
#include <sys/param.h>
#include <sys/user.h>
#include <sys/sysctl.h>

/**
 * Get process memory information for FreeBSD
 *
 * Uses the sysctl API to collect memory information.
 */
OS_PROCESS_MEMORY os_process_memory(pid_t pid) {
    OS_PROCESS_MEMORY proc_mem = OS_PROCESS_MEMORY_EMPTY;
    int mib[4];
    struct kinfo_proc proc;
    size_t len = sizeof(proc);
    
    if (pid == 0)
        pid = getpid();
    
    // Use direct sysctl call instead of higher-level functions
    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_PID;
    mib[3] = pid;
    
    if (sysctl(mib, 4, &proc, &len, NULL, 0) == 0) {
        int page_size = getpagesize();
        
        proc_mem.rss = proc.ki_rssize * page_size;
        proc_mem.virtual_size = proc.ki_size;
        
        // Get maximum RSS without using getrusage
        unsigned long maxrss = 0;
        size_t maxrss_len = sizeof(maxrss);
        char maxrss_name[128];
        size_t maxrss_name_len = 0;
        maxrss_name[0] = '\0';
        maxrss_name_len = strcatz(maxrss_name, maxrss_name_len, "kern.proc.", sizeof(maxrss_name));
        maxrss_name_len += print_uint64(&maxrss_name[maxrss_name_len], KERN_PROC_PID);
        maxrss_name_len = strcatz(maxrss_name, maxrss_name_len, ".rusage.", sizeof(maxrss_name));
        maxrss_name_len += print_uint64(&maxrss_name[maxrss_name_len], pid);
        maxrss_name_len = strcatz(maxrss_name, maxrss_name_len, ".maxrss", sizeof(maxrss_name));
        if (sysctlbyname(maxrss_name, &maxrss, &maxrss_len, NULL, 0) == 0)
            proc_mem.max_rss = maxrss * 1024; // maxrss is in KB
        else
            proc_mem.max_rss = proc_mem.rss; // Fall back to current RSS if peak not available
        
        // Get memory map information for shared memory approximation
        // This approach still uses syscall directly, which is already low-level
        size_t shared_pages_len = 0;
        int mib_shared[] = {CTL_KERN, KERN_PROC, KERN_PROC_VMMAP, pid};
        
        // First call to get size
        if (sysctl(mib_shared, 4, NULL, &shared_pages_len, NULL, 0) == 0 && shared_pages_len > 0) {
            // Allocate memory for the vmmap data
            void *shared_info = mallocz(shared_pages_len);
            if (sysctl(mib_shared, 4, shared_info, &shared_pages_len, NULL, 0) == 0) {
                // For now, use a simple approximation
                // A more accurate calculation would involve parsing the vm_map_entries
                proc_mem.shared = proc_mem.rss / 4; // rough estimate
            }
            freez(shared_info);
        }
    }
    
    if (OS_PROCESS_MEMORY_OK(proc_mem))
        last_process_memory_info = proc_mem;
        
    return proc_mem;
}
#endif

#if defined(OS_MACOS)
#include <mach/mach.h>
#include <mach/task.h>
#include <mach/mach_init.h>

/**
 * Get process memory information for macOS
 *
 * Uses the Mach task API to collect memory information.
 * Mach API is already a low-level API for accessing process information on macOS.
 */
OS_PROCESS_MEMORY os_process_memory(pid_t pid) {
    OS_PROCESS_MEMORY proc_mem = OS_PROCESS_MEMORY_EMPTY;
    task_t task;
    kern_return_t kr;
    
    if (pid == 0)
        pid = getpid();
    
    // Get the Mach task for the process - this is a low-level system call
    kr = task_for_pid(mach_task_self(), pid, &task);
    if (kr != KERN_SUCCESS) {
        if (pid == getpid()) {
            // Always works for current process
            task = mach_task_self();
        } else {
            // Can't get info for other processes without privileges
            return proc_mem;
        }
    }
    
    // Get basic task info (RSS and virtual size) - direct Mach API call
    struct task_basic_info_64 task_basic_info;
    mach_msg_type_number_t count = TASK_BASIC_INFO_64_COUNT;
    kr = task_info(task, TASK_BASIC_INFO_64, (task_info_t)&task_basic_info, &count);
    
    if (kr == KERN_SUCCESS) {
        proc_mem.rss = task_basic_info.resident_size;
        proc_mem.virtual_size = task_basic_info.virtual_size;
        
        // Get page-in information to estimate max RSS - direct Mach API call
        // This avoids using getrusage().ru_maxrss
        task_events_info_data_t events_info;
        mach_msg_type_number_t events_info_count = TASK_EVENTS_INFO_COUNT;
        kr = task_info(task, TASK_EVENTS_INFO, (task_info_t)&events_info, &events_info_count);
        
        if (kr == KERN_SUCCESS && events_info.pageins > 0) {
            // If we have page-ins, we can use a more accurate calculation
            vm_size_t page_size = 0;  // Changed from mach_vm_size_t to vm_size_t
            host_page_size(mach_host_self(), &page_size);
            proc_mem.max_rss = task_basic_info.resident_size + (events_info.pageins * page_size);
        } else {
            // Otherwise, just use current RSS as a fallback
            proc_mem.max_rss = task_basic_info.resident_size;
        }
        
        // On macOS, use a simpler approach without vm_region
        // Just estimate shared, text, and data based on total memory
        if (task_basic_info.resident_size > 0) {
            // Simple heuristic estimates based on typical process memory layout
            proc_mem.shared = task_basic_info.resident_size / 5;  // ~20% shared libraries
            proc_mem.text = task_basic_info.resident_size / 5;    // ~20% code
            proc_mem.data = task_basic_info.resident_size - proc_mem.shared - proc_mem.text; // remaining is data
        }
    }
    
    // Release the task port if we obtained it - low-level resource management
    if (task != mach_task_self()) {
        mach_port_deallocate(mach_task_self(), task);
    }
    
    if (OS_PROCESS_MEMORY_OK(proc_mem))
        last_process_memory_info = proc_mem;
        
    return proc_mem;
}
#endif

#if defined(OS_WINDOWS)
#include <windows.h>
#include <psapi.h>
#include <tlhelp32.h>

/**
 * Get process memory information for Windows
 *
 * Uses Windows APIs to collect memory information.
 * Windows API is inherently lower-level compared to standard C library functions.
 */
OS_PROCESS_MEMORY os_process_memory(pid_t pid) {
    OS_PROCESS_MEMORY proc_mem = OS_PROCESS_MEMORY_EMPTY;
    HANDLE hProcess;
    DWORD process_id = pid;
    
    if (process_id == 0)
        process_id = GetCurrentProcessId();
    
    // OpenProcess is a direct Windows API call - low level access to process
    hProcess = OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, FALSE, process_id);
    if (hProcess) {
        // GetProcessMemoryInfo is a direct Windows API call to get memory info
        PROCESS_MEMORY_COUNTERS_EX pmc;
        ZeroMemory(&pmc, sizeof(pmc));
        pmc.cb = sizeof(pmc);
        
        if (GetProcessMemoryInfo(hProcess, (PROCESS_MEMORY_COUNTERS*)&pmc, sizeof(pmc))) {
            proc_mem.rss = pmc.WorkingSetSize;
            proc_mem.max_rss = pmc.PeakWorkingSetSize;
            proc_mem.virtual_size = pmc.PagefileUsage + pmc.WorkingSetSize;
            
            // Get module information to determine text and shared memory
            // CreateToolhelp32Snapshot is a low-level Windows API call
            HANDLE hSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPMODULE | TH32CS_SNAPMODULE32, process_id);
            if (hSnapshot != INVALID_HANDLE_VALUE) {
                MODULEENTRY32 me;
                ZeroMemory(&me, sizeof(me));
                me.dwSize = sizeof(MODULEENTRY32);
                
                // Module32First/Next are direct API calls to examine loaded modules
                if (Module32First(hSnapshot, &me)) {
                    // The first module is the executable itself
                    proc_mem.text = me.modBaseSize;
                    
                    // Sum all other modules as shared code
                    while (Module32Next(hSnapshot, &me)) {
                        proc_mem.shared += me.modBaseSize;
                    }
                }
                CloseHandle(hSnapshot);
            }
            
            // Get virtual memory information for more detailed breakdown
            MEMORY_BASIC_INFORMATION mbi;
            ZeroMemory(&mbi, sizeof(mbi));
            SIZE_T address = 0;
            
            // VirtualQueryEx is a low-level API to get memory region info
            while (VirtualQueryEx(hProcess, (LPCVOID)address, &mbi, sizeof(mbi)) == sizeof(mbi)) {
                if (mbi.State == MEM_COMMIT) {
                    if (mbi.Type == MEM_PRIVATE && !(mbi.Protect & (PAGE_EXECUTE | PAGE_EXECUTE_READ | PAGE_EXECUTE_READWRITE | PAGE_EXECUTE_WRITECOPY))) {
                        // Private data memory
                        proc_mem.data += mbi.RegionSize;
                    }
                }
                
                // Move to the next region
                address = (SIZE_T)mbi.BaseAddress + mbi.RegionSize;
                
                // Avoid potential infinite loop on 64-bit systems
                if (address < (SIZE_T)mbi.BaseAddress)
                    break;
            }
            
            // If data counting failed, fall back to estimation
            if (proc_mem.data == 0)
                proc_mem.data = pmc.PrivateUsage - proc_mem.text;
        }
        CloseHandle(hProcess);
    }
    
    if (OS_PROCESS_MEMORY_OK(proc_mem))
        last_process_memory_info = proc_mem;
        
    return proc_mem;
}
#endif