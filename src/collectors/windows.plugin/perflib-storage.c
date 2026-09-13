// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME PLUGIN_WINDOWS_NAME
#define _COMMON_PLUGIN_MODULE_NAME "PerflibStorage"
#define CONFIG_SECTION_PERFLIB_STORAGE "plugin:windows:PerflibStorage"
#ifndef IO_REPARSE_TAG_CSV
#define IO_REPARSE_TAG_CSV ((DWORD)0x80000009UL)
#endif
#include "../common-contexts/common-contexts.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"

struct logical_disk {
    usec_t last_collected;
    bool collected_metadata;
    usec_t metadata_retry_after;

    UINT DriveType;
    DWORD SerialNumber;
    ULONG divisor;
    ULONG chart_divisor; // the divisor the dimensions were created with
    bool readonly;

    STRING *filesystem;

    RRDSET *st_disk_space;
    RRDDIM *rd_disk_space_used;
    RRDDIM *rd_disk_space_free;

    COUNTER_DATA percentDiskFree;
    // COUNTER_DATA freeMegabytes;
};

struct physical_disk {
    usec_t last_collected;
    bool collected_metadata;

    STRING *device;
    STRING *mount_point;
    STRING *manufacturer;
    STRING *model;
    STRING *media_type;
    STRING *name;
    STRING *device_id;

    ND_DISK_IO disk_io;
    // COUNTER_DATA diskBytesPerSec;
    COUNTER_DATA diskReadBytesPerSec;
    COUNTER_DATA diskWriteBytesPerSec;

    ND_DISK_OPS disk_ops;
    // COUNTER_DATA diskTransfersPerSec;
    COUNTER_DATA diskReadsPerSec;
    COUNTER_DATA diskWritesPerSec;

    ND_DISK_UTIL disk_util;
    COUNTER_DATA percentIdleTime;

    ND_DISK_BUSY disk_busy;
    COUNTER_DATA percentDiskTime;

    ND_DISK_IOTIME disk_iotime;
    COUNTER_DATA percentDiskReadTime;
    COUNTER_DATA percentDiskWriteTime;

    ND_DISK_QOPS disk_qops;
    COUNTER_DATA currentDiskQueueLength;
    // COUNTER_DATA averageDiskQueueLength;
    // COUNTER_DATA averageDiskReadQueueLength;
    // COUNTER_DATA averageDiskWriteQueueLength;

    ND_DISK_AWAIT disk_await;
    COUNTER_DATA averageDiskSecondsPerRead;
    COUNTER_DATA averageDiskSecondsPerWrite;

    ND_DISK_SVCTM disk_svctm;
    COUNTER_DATA averageDiskSecondsPerTransfer;

    ND_DISK_AVGSZ disk_avgsz;
    //COUNTER_DATA averageDiskBytesPerTransfer;
    COUNTER_DATA averageDiskBytesPerRead;
    COUNTER_DATA averageDiskBytesPerWrite;

    COUNTER_DATA splitIoPerSec;
    RRDSET *st_split;
    RRDDIM *rd_split;
};

struct physical_disk system_physical_total = {
    .collected_metadata = true,
};

static void
dict_logical_disk_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct logical_disk *d = value;

    d->percentDiskFree.key = "% Free Space";
    // d->freeMegabytes.key = "Free Megabytes";
}

static void logical_disk_cleanup(struct logical_disk *d)
{
    string_freez(d->filesystem);
    d->filesystem = NULL;

    d->collected_metadata = false;
    d->metadata_retry_after = 0;

    rrdset_is_obsolete___safe_from_collector_thread(d->st_disk_space);
    d->st_disk_space = NULL;
}

static void physical_disk_initialize(struct physical_disk *d)
{
    d->percentIdleTime.key = "% Idle Time";
    d->percentDiskTime.key = "% Disk Time";
    d->percentDiskReadTime.key = "% Disk Read Time";
    d->percentDiskWriteTime.key = "% Disk Write Time";
    d->currentDiskQueueLength.key = "Current Disk Queue Length";
    // d->averageDiskQueueLength.key = "Avg. Disk Queue Length";
    // d->averageDiskReadQueueLength.key = "Avg. Disk Read Queue Length";
    // d->averageDiskWriteQueueLength.key = "Avg. Disk Write Queue Length";
    d->averageDiskSecondsPerTransfer.key = "Avg. Disk sec/Transfer";
    d->averageDiskSecondsPerRead.key = "Avg. Disk sec/Read";
    d->averageDiskSecondsPerWrite.key = "Avg. Disk sec/Write";
    // d->diskTransfersPerSec.key = "Disk Transfers/sec";
    d->diskReadsPerSec.key = "Disk Reads/sec";
    d->diskWritesPerSec.key = "Disk Writes/sec";
    // d->diskBytesPerSec.key = "Disk Bytes/sec";
    d->diskReadBytesPerSec.key = "Disk Read Bytes/sec";
    d->diskWriteBytesPerSec.key = "Disk Write Bytes/sec";
    // d->averageDiskBytesPerTransfer.key = "Avg. Disk Bytes/Transfer";
    d->averageDiskBytesPerRead.key = "Avg. Disk Bytes/Read";
    d->averageDiskBytesPerWrite.key = "Avg. Disk Bytes/Write";
    d->splitIoPerSec.key = "Split IO/Sec";
}

static void physical_disk_cleanup(struct physical_disk *d)
{
    string_freez(d->device);
    d->device = NULL;
    string_freez(d->mount_point);
    d->mount_point = NULL;
    string_freez(d->manufacturer);
    d->manufacturer = NULL;
    string_freez(d->model);
    d->model = NULL;
    string_freez(d->media_type);
    d->media_type = NULL;
    string_freez(d->name);
    d->name = NULL;
    string_freez(d->device_id);
    d->device_id = NULL;

    d->collected_metadata = false;

    rrdset_is_obsolete___safe_from_collector_thread(d->disk_io.st_io);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_ops.st_ops);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_util.st_util);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_busy.st_busy);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_iotime.st_iotime);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_qops.st_qops);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_await.st_await);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_svctm.st_svctm);
    rrdset_is_obsolete___safe_from_collector_thread(d->disk_avgsz.st_avgsz);
    rrdset_is_obsolete___safe_from_collector_thread(d->st_split);
}

void dict_physical_disk_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct physical_disk *pd = value;
    physical_disk_initialize(pd);
}

static void dict_logical_disk_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct logical_disk *d = value;
    logical_disk_cleanup(d);
}

static void dict_physical_disk_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct physical_disk *d = value;
    physical_disk_cleanup(d);
}

static DICTIONARY *logicalDisks = NULL, *physicalDisks = NULL;
static DICTIONARY *mountPoints = NULL, *deviceMountPaths = NULL;
static DICTIONARY *excludedDeviceNames = NULL;
static usec_t mountPointsRefreshedUT = 0;
static SIMPLE_PATTERN *excluded_logical_disk_paths = NULL;

struct volume_space_result {
    bool success;
    uint64_t total_bytes;
    uint64_t free_bytes;
};

static netdata_mutex_t volume_space_mutex;
static netdata_cond_t volume_space_cond;
static DICTIONARY *volume_space_request = NULL;
static DICTIONARY *volume_space_results = NULL;
static bool volume_space_worker_stop = false;
static ND_THREAD *volume_space_thread = NULL;
static bool storage_initialized = false;

static inline bool logical_disk_is_excluded(const char *mount_point)
{
    return (excluded_logical_disk_paths && simple_pattern_matches(excluded_logical_disk_paths, mount_point)) ||
           (excludedDeviceNames && dictionary_get(excludedDeviceNames, mount_point) != NULL);
}

static void volume_space_worker(void *ptr);

static void initialize(void)
{
    physical_disk_initialize(&system_physical_total);

    excluded_logical_disk_paths = simple_pattern_create(
        inicfg_get(&netdata_config, CONFIG_SECTION_PERFLIB_STORAGE, "exclude space metrics on paths", ""),
        NULL,
        SIMPLE_PATTERN_EXACT,
        false);

    logicalDisks = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct logical_disk));

    dictionary_register_insert_callback(logicalDisks, dict_logical_disk_insert_cb, NULL);
    dictionary_register_delete_callback(logicalDisks, dict_logical_disk_delete_cb, NULL);

    physicalDisks = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct physical_disk));

    dictionary_register_insert_callback(physicalDisks, dict_physical_disk_insert_cb, NULL);
    dictionary_register_delete_callback(physicalDisks, dict_physical_disk_delete_cb, NULL);

    mountPoints = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    deviceMountPaths = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    excludedDeviceNames = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    netdata_mutex_init(&volume_space_mutex);
    netdata_cond_init(&volume_space_cond);
    volume_space_thread = nd_thread_create(
        "WIN[PerflibStorage space]", NETDATA_THREAD_OPTION_DEFAULT, volume_space_worker, NULL);
    storage_initialized = true;
}

// --------------------------------------------------------------------------------------------------------------------
// mount point registry
//
// The perflib LogicalDisk object cannot enumerate every volume worth reporting. CSVs do not have a
// drive letter or LogicalDisk instance; Windows exposes their stable per-node access path below
// %SystemDrive%\ClusterStorage. Volumes mounted into a folder may also be published by perflib
// under their bare device name (HarddiskVolumeN), which an operator cannot map back to anything.
//
// So we keep our own registry of mount paths, refreshed on an interval, and use it both to resolve
// perflib device names and to publish the volumes perflib never lists.

// Long enough for the extended-length paths Windows accepts, in UTF-8 bytes.
#define ND_MOUNT_PATH_MAX 1024

// Volumes and CSVs do not come and go often; rescanning every collection would be pure syscall cost.
#define MOUNT_POINTS_REFRESH_EVERY_UT (60 * USEC_PER_SEC)
#define METADATA_RETRY_EVERY_UT (60 * USEC_PER_SEC)

// Canonical form of a mount path: no trailing backslash, so "C:\" and "C:" are the same instance.
// path_size bounds the scan so a non-null-terminated buffer cannot run past the end of the array.
static void canonicalize_mount_path(char *path, size_t path_size)
{
    size_t len = strnlen(path, path_size);
    while (len > 1 && path[len - 1] == '\\')
        path[--len] = '\0';
}

static bool mount_path_is_drive_letter(const char *path)
{
    return path && isalpha((uint8_t)path[0]) && path[1] == ':' && path[2] == '\0';
}

static void mount_points_choose_path(char *first_path, size_t first_path_size, const char *path)
{
    bool path_is_drive = mount_path_is_drive_letter(path);
    bool first_is_drive = mount_path_is_drive_letter(first_path);

    if (!*first_path || (path_is_drive && !first_is_drive) ||
        (path_is_drive == first_is_drive && strcasecmp(path, first_path) < 0))
        snprintfz(first_path, first_path_size, "%s", path);
}

// Build the Win32 root path used to query a volume:
//   "C:"                      -> "C:\"
//   "C:\ClusterStorage\Volume1" -> "C:\ClusterStorage\Volume1\"
//   "HarddiskVolume7"         -> "\\.\HarddiskVolume7\"
static bool volume_root_path_w(const char *name, wchar_t *dst, size_t dst_count)
{
    if (!name || !*name || !dst || dst_count < 2)
        return false;

    char path[ND_MOUNT_PATH_MAX];
    size_t name_len = strlen(name);
    bool direct_path = (isalpha((uint8_t)name[0]) && name[1] == ':') || name[0] == '\\';
    if ((direct_path && name_len >= sizeof(path)) || (!direct_path && name_len + 4 >= sizeof(path)))
        return false;

    if (direct_path)
        snprintfz(path, sizeof(path), "%s", name);
    else
        snprintfz(path, sizeof(path), "\\\\.\\%s", name);

    size_t len = strnlen(path, sizeof(path));
    if (len && path[len - 1] != '\\' && len + 2 <= sizeof(path)) {
        path[len] = '\\';
        path[len + 1] = '\0';
    }

    bool truncated = false;
    if (any_to_utf16(CP_UTF8, dst, dst_count, path, -1, &truncated) == 0 || truncated)
        return false;

    return true;
}

// Total and free bytes of the volume the given path resolves to. This follows mount points, which
// is what makes CSVs readable: the CSVFS driver answers for C:\ClusterStorage\VolumeN.
static bool volume_space(const char *name, uint64_t *total_bytes, uint64_t *free_bytes)
{
    wchar_t rootPath[ND_MOUNT_PATH_MAX];
    if (!volume_root_path_w(name, rootPath, _countof(rootPath)))
        return false;

    // Description of incompatibilities present in both methods we are using
    // https://devblogs.microsoft.com/oldnewthing/20071101-00/?p=24613
    // We are using the variable that should not be affected by quotas.
    //
    // GetDiskFreeSpaceEx() failing is the only reliable "this volume cannot be read" signal: a CSV
    // mount point is not a drive root, so pre-screening it with GetDriveType() would discard the
    // very volumes this pass exists to collect.
    ULARGE_INTEGER totalAvailableToCaller, totalNumberOfBytes, totalNumberOfFreeBytes;
    if (!GetDiskFreeSpaceExW(rootPath, &totalAvailableToCaller, &totalNumberOfBytes, &totalNumberOfFreeBytes))
        return false;

    *total_bytes = totalNumberOfBytes.QuadPart;
    *free_bytes = totalNumberOfFreeBytes.QuadPart;
    return true;
}

static void volume_space_worker(void *ptr __maybe_unused)
{
    worker_register("WIN_PERFLIB_STORAGE_SPACE");

    DWORD previous_error_mode = 0;
    SetThreadErrorMode(SEM_FAILCRITICALERRORS, &previous_error_mode);

    while (service_running(SERVICE_COLLECTORS)) {
        netdata_mutex_lock(&volume_space_mutex);
        while (!volume_space_worker_stop && !volume_space_request)
            netdata_cond_wait(&volume_space_cond, &volume_space_mutex);

        if (volume_space_worker_stop) {
            netdata_mutex_unlock(&volume_space_mutex);
            break;
        }

        DICTIONARY *request = volume_space_request;
        volume_space_request = NULL;
        netdata_mutex_unlock(&volume_space_mutex);

        DICTIONARY *results = dictionary_create_advanced(
            DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct volume_space_result));

        struct volume_space_result *result;
        dfe_start_read(request, result)
        {
            struct volume_space_result value = {0};
            value.success = volume_space(v_dfe.name, &value.total_bytes, &value.free_bytes);
            dictionary_set(results, v_dfe.name, &value, sizeof(value));
        }
        dfe_done(result);
        dictionary_destroy(request);

        netdata_mutex_lock(&volume_space_mutex);
        dictionary_destroy(volume_space_results);
        volume_space_results = results;
        netdata_cond_broadcast(&volume_space_cond);
        netdata_mutex_unlock(&volume_space_mutex);
    }

    SetThreadErrorMode(previous_error_mode, NULL);
    worker_unregister();
}

static void volume_space_submit(void)
{
    DICTIONARY *request = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    void *value;
    dfe_start_read(mountPoints, value)
    {
        dictionary_set(request, v_dfe.name, NULL, 0);
    }
    dfe_done(value);

    netdata_mutex_lock(&volume_space_mutex);
    if (!volume_space_worker_stop) {
        // Keep only the newest request while a slow query is in progress. The worker will pick it
        // up after the current request completes, so a busy worker never causes a cycle to be lost.
        dictionary_destroy(volume_space_request);
        volume_space_request = request;
        netdata_cond_signal(&volume_space_cond);
        request = NULL;
    }
    netdata_mutex_unlock(&volume_space_mutex);

    dictionary_destroy(request);
}

static DICTIONARY *volume_space_collect_results(void)
{
    netdata_mutex_lock(&volume_space_mutex);
    DICTIONARY *results = volume_space_results;
    volume_space_results = NULL;
    netdata_mutex_unlock(&volume_space_mutex);
    return results;
}

// Register one canonical mount path per volume; prefer a drive letter because PerfLib commonly
// reports that alias, while returning every alias would create duplicate full-capacity charts.
static bool mount_points_add_volume_paths(
    DICTIONARY *paths,
    const wchar_t *volumeGUID,
    char *first_path,
    size_t first_path_size,
    bool *had_path)
{
    wchar_t stack_buf[512];
    wchar_t *buf = stack_buf;
    DWORD buf_count = _countof(stack_buf);
    DWORD needed = 0;

    if (!GetVolumePathNamesForVolumeNameW(volumeGUID, buf, buf_count, &needed)) {
        if (GetLastError() != ERROR_MORE_DATA || !needed)
            return false;

        buf = mallocz((size_t)needed * sizeof(wchar_t));
        buf_count = needed;

        if (!GetVolumePathNamesForVolumeNameW(volumeGUID, buf, buf_count, &needed)) {
            freez(buf);
            return false;
        }
    }

    // the result is a MULTI_SZ: every mount path of the volume, terminated by an empty string
    for (const wchar_t *p = buf; p < buf + buf_count && *p;) {
        // bound the scan so a malformed entry that runs past the buffer cannot be walked off the end
        size_t remaining = (size_t)buf_count - (size_t)(p - buf);
        size_t entry_len = wcsnlen(p, remaining);
        if (entry_len == remaining)
            break;

        // Advance past this entry before the rest of the body runs. Everything below leaves the
        // iteration with continue, and the loop has no increment in its header, so an increment
        // placed after those skips would never run for a skipped entry and the walk would spin on
        // it forever - a hang, not a missed volume.
        const wchar_t *entry = p;
        p += entry_len + 1;

        char path[ND_MOUNT_PATH_MAX];
        bool truncated = false;
        if (!utf16_to_utf8(path, sizeof(path), entry, (int)entry_len, &truncated) || truncated)
            continue;

        canonicalize_mount_path(path, sizeof(path));
        if (!*path)
            continue;

        if (had_path)
            *had_path = true;

        if (logical_disk_is_excluded(path))
            continue;

        if (first_path)
            mount_points_choose_path(first_path, first_path_size, path);
    }

    if (buf != stack_buf)
        freez(buf);

    if (first_path && *first_path)
        dictionary_set(paths, first_path, NULL, 0);

    return true;
}

// Map the volume's NT device name (\Device\HarddiskVolumeN) to a mount path, so a perflib instance
// named after the device can be published under a name operators recognize.
static void mount_points_map_device(
    DICTIONARY *devices, DICTIONARY *excluded_devices, const wchar_t *volumeGUID, const char *mount_path)
{
    // "\\?\Volume{guid}\" -> "Volume{guid}"
    // FindFirstVolumeW() always null-terminates, but bound the scan to the caller's buffer so a
    // future change that passes a different size cannot walk past the end of the array.
    size_t len = wcsnlen(volumeGUID, MAX_PATH + 1);
    if (len < 5 || wcsncmp(volumeGUID, L"\\\\?\\", 4) != 0)
        return;

    wchar_t name[MAX_PATH + 1];
    size_t name_len = len - 4;
    if (name_len >= _countof(name))
        return;

    memcpy(name, volumeGUID + 4, name_len * sizeof(wchar_t));
    name[name_len] = L'\0';
    while (name_len && name[name_len - 1] == L'\\')
        name[--name_len] = L'\0';

    wchar_t target[MAX_PATH + 1];
    if (!QueryDosDeviceW(name, target, _countof(target)))
        return;

    // perflib publishes the leaf of the device name, e.g. "HarddiskVolume7"
    const wchar_t *leaf = wcsrchr(target, L'\\');
    leaf = leaf ? leaf + 1 : target;

    char device[128];
    bool truncated = false;
    if (!utf16_to_utf8(device, sizeof(device), leaf, -1, &truncated) || truncated || !*device)
        return;

    if (mount_path && *mount_path)
        dictionary_set(devices, device, (void *)mount_path, strlen(mount_path) + 1);
    else if (excluded_devices)
        dictionary_set(excluded_devices, device, device, strlen(device) + 1);
}

static void mount_points_mark_device_excluded(DICTIONARY *excluded_devices, const wchar_t *volumeGUID)
{
    mount_points_map_device(NULL, excluded_devices, volumeGUID, NULL);
}

// Returns false when the enumeration could not be started at all, i.e. we learned nothing and the
// caller must keep the snapshot it already has.
static bool mount_points_scan_volumes(DICTIONARY *paths, DICTIONARY *devices, DICTIONARY *excluded_devices)
{
    wchar_t volumeGUID[MAX_PATH + 1];

    HANDLE h = FindFirstVolumeW(volumeGUID, _countof(volumeGUID));
    if (h == INVALID_HANDLE_VALUE) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_DEBUG,
            "FindFirstVolumeW() failed (error: %lu); keeping the previous mount point registry",
            GetLastError());
        return false;
    }

    do {
        char first_path[ND_MOUNT_PATH_MAX] = "";
        bool had_path = false;
        if (!mount_points_add_volume_paths(paths, volumeGUID, first_path, sizeof(first_path), &had_path)) {
            FindVolumeClose(h);
            return false;
        }

        // a volume with no mount path is not reachable, so there is nothing to name it after
        if (*first_path)
            mount_points_map_device(devices, excluded_devices, volumeGUID, first_path);
        else if (had_path)
            mount_points_mark_device_excluded(excluded_devices, volumeGUID);

    } while (FindNextVolumeW(h, volumeGUID, _countof(volumeGUID)));

    // capture before FindVolumeClose(), which overwrites the thread's last error
    DWORD err = GetLastError();
    FindVolumeClose(h);

    // anything other than a clean end of enumeration means the snapshot is incomplete; accepting it
    // would evict the volumes we never reached
    if (err != ERROR_NO_MORE_FILES) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_DEBUG,
            "FindNextVolumeW() failed (error: %lu); keeping the previous mount point registry",
            err);
        return false;
    }

    return true;
}

// Cluster Shared Volumes have their stable per-node access paths below %SystemDrive%\ClusterStorage.
// Scan that directory in addition to regular volume mount points, which need not enumerate CSVs.
// Returns false only on an unexpected failure. A host with no \ClusterStorage directory is the
// normal non-cluster case and counts as a successful scan that found no CSVs - treating it as a
// failure would freeze the registry permanently on every non-cluster Windows host.
static bool mount_points_scan_cluster_storage(DICTIONARY *paths)
{
    wchar_t windir[MAX_PATH + 1];
    UINT len = GetSystemWindowsDirectoryW(windir, _countof(windir));
    if (!len || len >= _countof(windir) || windir[1] != L':' ||
        !((windir[0] >= L'A' && windir[0] <= L'Z') || (windir[0] >= L'a' && windir[0] <= L'z')))
        return false;

    static const wchar_t suffix[] = L":\\ClusterStorage\\*";
    wchar_t pattern[_countof(suffix) + 1];
    pattern[0] = windir[0];
    memcpy(&pattern[1], suffix, sizeof(suffix));

    WIN32_FIND_DATAW fd;
    HANDLE h = FindFirstFileW(pattern, &fd);
    if (h == INVALID_HANDLE_VALUE) {
        DWORD err = GetLastError();
        if (err == ERROR_FILE_NOT_FOUND || err == ERROR_PATH_NOT_FOUND)
            return true; // not a cluster node, or no CSVs mounted

        nd_log(
            NDLS_COLLECTORS,
            NDLP_DEBUG,
            "Cannot enumerate ClusterStorage (error: %lu); keeping the previous mount point registry",
            err);
        return false;
    }

    do {
        if (!(fd.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY))
            continue;

        if ((fd.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT) &&
            fd.dwReserved0 != IO_REPARSE_TAG_MOUNT_POINT && fd.dwReserved0 != IO_REPARSE_TAG_CSV)
            continue;

        if (fd.cFileName[0] == L'.' &&
            (fd.cFileName[1] == L'\0' || (fd.cFileName[1] == L'.' && fd.cFileName[2] == L'\0')))
            continue;

        char name[ND_MOUNT_PATH_MAX];
        bool truncated = false;
        if (!utf16_to_utf8(name, sizeof(name), fd.cFileName, -1, &truncated) || truncated || !*name)
            continue;

        char path[ND_MOUNT_PATH_MAX];
        static const char prefix[] = ":\\ClusterStorage\\";
        if (strlen(name) + sizeof(prefix) + 1 > sizeof(path))
            continue;
        snprintfz(path, sizeof(path), "%c:\\ClusterStorage\\%s", (char)windir[0], name);

        if ((fd.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT) &&
            fd.dwReserved0 == IO_REPARSE_TAG_MOUNT_POINT) {
            wchar_t mount_path[ND_MOUNT_PATH_MAX];
            int mount_path_len = swprintf(
                mount_path, _countof(mount_path), L"%c:\\ClusterStorage\\%ls\\", windir[0], fd.cFileName);
            wchar_t volume_name[MAX_PATH + 1];
            if (mount_path_len < 0 || (size_t)mount_path_len >= _countof(mount_path) ||
                !GetVolumeNameForVolumeMountPointW(mount_path, volume_name, _countof(volume_name)))
                continue;
        }

        if (!logical_disk_is_excluded(path))
            dictionary_set(paths, path, NULL, 0);

    } while (FindNextFileW(h, &fd));

    // capture before FindClose(), which overwrites the thread's last error
    DWORD err = GetLastError();
    FindClose(h);

    if (err != ERROR_NO_MORE_FILES) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_DEBUG,
            "Cannot finish enumerating ClusterStorage (error: %lu); keeping the previous mount point registry",
            err);
        return false;
    }

    return true;
}

struct mount_points_scan_ops {
    bool (*scan_volumes)(DICTIONARY *paths, DICTIONARY *devices, DICTIONARY *excluded_devices);
    bool (*scan_cluster_storage)(DICTIONARY *paths);
};

static const struct mount_points_scan_ops mount_points_production_scan_ops = {
    .scan_volumes = mount_points_scan_volumes,
    .scan_cluster_storage = mount_points_scan_cluster_storage,
};

// The registry is a snapshot - volumes and CSVs are added and removed while we run - but it is
// rebuilt into fresh dictionaries and swapped in only once the scan actually produced one.
// Discarding the old snapshot first would make a transient enumeration failure indistinguishable
// from "every volume disappeared": mount-point-only instances (CSVs above all) would be evicted by
// logical_disk_evict_stale() and their charts obsoleted, and perflib instances that resolve through
// the device map would flip to their bare HarddiskVolumeN name and back a minute later.
static void mount_points_refresh_with_ops(usec_t now_ut, const struct mount_points_scan_ops *ops)
{
    if (mountPointsRefreshedUT && now_ut - mountPointsRefreshedUT < MOUNT_POINTS_REFRESH_EVERY_UT)
        return;

    // back off on failure too: retrying a failing enumeration every second would cost more than the
    // stale snapshot it is trying to replace
    mountPointsRefreshedUT = now_ut;

    DICTIONARY *paths = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    DICTIONARY *devices = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    DICTIONARY *excluded_devices = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    // both scans must run: neither short-circuits the other
    bool volumes_ok = ops->scan_volumes(paths, devices, excluded_devices);
    bool cluster_ok = ops->scan_cluster_storage(paths);

    if (!volumes_ok || !cluster_ok) {
        // a partially built snapshot is worse than a slightly stale one
        dictionary_destroy(paths);
        dictionary_destroy(devices);
        dictionary_destroy(excluded_devices);
        return;
    }

    dictionary_destroy(mountPoints);
    dictionary_destroy(deviceMountPaths);
    dictionary_destroy(excludedDeviceNames);
    mountPoints = paths;
    deviceMountPaths = devices;
    excludedDeviceNames = excluded_devices;
}

static void mount_points_refresh(usec_t now_ut)
{
    mount_points_refresh_with_ops(now_ut, &mount_points_production_scan_ops);
}

// Perflib names a letterless volume after its device (HarddiskVolumeN). Publish it under a mount
// path when it has one, so the instance is recognizable and its metadata can be queried.
static void logical_disk_resolve_name(const char *instance, char *dst, size_t dst_size)
{
    if (!strpbrk(instance, ":\\")) {
        const char *mount_path = dictionary_get(deviceMountPaths, instance);
        if (mount_path && *mount_path) {
            snprintfz(dst, dst_size, "%s", mount_path);
            return;
        }
    }

    snprintfz(dst, dst_size, "%s", instance);
    canonicalize_mount_path(dst, dst_size);
}

// Volume metadata: filesystem, serial number and read-only state. The perflib instance name is
// UTF-8, so the wide APIs are the only correct way to pass it back to Windows.
static STRING *getFileSystemType(struct logical_disk *d, const char *diskName)
{
    wchar_t rootPath[ND_MOUNT_PATH_MAX];
    if (!volume_root_path_w(diskName, rootPath, _countof(rootPath)))
        return NULL;

    d->DriveType = GetDriveTypeW(rootPath);

    wchar_t volumeName[MAX_PATH + 1] = {0};
    wchar_t fileSystemName[64] = {0};
    DWORD serialNumber = 0;
    DWORD maxComponentLength = 0;
    DWORD fileSystemFlags = 0;

    if (!GetVolumeInformationW(
            rootPath,
            volumeName,
            _countof(volumeName),
            &serialNumber,
            &maxComponentLength,
            &fileSystemFlags,
            fileSystemName,
            _countof(fileSystemName)))
        return NULL;

    d->readonly = fileSystemFlags & FILE_READ_ONLY_VOLUME;
    d->SerialNumber = serialNumber;

    char fileSystem[128];
    bool truncated = false;
    if (!utf16_to_utf8(fileSystem, sizeof(fileSystem), fileSystemName, -1, &truncated) || truncated || !*fileSystem)
        return NULL;

    for (char *c = fileSystem; *c; c++)
        *c = (char)tolower((uint8_t)*c);

    return string_strdupz(fileSystem);
}

static void logical_disk_collect_metadata(struct logical_disk *d, const char *name, usec_t now_ut)
{
    if (d->collected_metadata || (d->metadata_retry_after && now_ut < d->metadata_retry_after))
        return;

    d->filesystem = getFileSystemType(d, name);
    if (d->filesystem) {
        d->collected_metadata = true;
        d->metadata_retry_after = 0;
    } else {
        d->metadata_retry_after = now_ut + METADATA_RETRY_EVERY_UT;
    }
}

static const char *drive_type_to_str(UINT type)
{
    switch (type) {
        default:
        case 0:
            return "unknown";
        case 1:
            return "norootdir";
        case 2:
            return "removable";
        case 3:
            return "fixed";
        case 4:
            return "remote";
        case 5:
            return "cdrom";
        case 6:
            return "ramdisk";
    }
}

// Space for a perflib instance. The Win32 APIs are authoritative and report bytes; the perflib
// "% Free Space" counter (free/total in MiB) is only the fallback for volumes they cannot open.
typedef bool (*volume_space_fn)(const char *name, uint64_t *total_bytes, uint64_t *free_bytes);

static void logical_disk_set_space_with_ops(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct logical_disk *d,
    const char *name,
    volume_space_fn space)
{
    uint64_t total_bytes, free_bytes;

    if (!space(name, &total_bytes, &free_bytes)) {
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskFree);
        d->divisor = 1024;
        return;
    }

    d->divisor = GIGA_FACTOR;
    d->percentDiskFree.current.Data = (ULONGLONG)free_bytes;
    d->percentDiskFree.current.Time = (LONGLONG)total_bytes;
}

static void logical_disk_set_space(PERF_DATA_BLOCK *pDataBlock,
                                   PERF_OBJECT_TYPE *pObjectType,
                                   PERF_INSTANCE_DEFINITION *pi,
                                   struct logical_disk *d,
                                   const char *name)
{
    logical_disk_set_space_with_ops(pDataBlock, pObjectType, pi, d, name, volume_space);
}

// percentDiskFree carries the free space in Data and the size of the volume in Time, both scaled by
// divisor. Shared by the perflib pass and the mount point pass so every volume looks the same.
static void logical_disk_chart(struct logical_disk *d, const char *name, int update_every)
{
    if (!d->st_disk_space) {
        d->st_disk_space = rrdset_create_localhost(
            "disk_space",
            name,
            NULL,
            name,
            "disk.space",
            "Disk Space Usage",
            "GiB",
            PLUGIN_WINDOWS_NAME,
            "PerflibStorage",
            NETDATA_CHART_PRIO_DISKSPACE_SPACE,
            update_every,
            RRDSET_TYPE_STACKED);

        rrdlabels_add(d->st_disk_space->rrdlabels, "mount_point", name, RRDLABEL_SRC_AUTO);
        rrdlabels_add(d->st_disk_space->rrdlabels, "drive_type", drive_type_to_str(d->DriveType), RRDLABEL_SRC_AUTO);
        rrdlabels_add(
            d->st_disk_space->rrdlabels,
            "filesystem",
            d->filesystem ? string2str(d->filesystem) : "unknown",
            RRDLABEL_SRC_AUTO);
        rrdlabels_add(d->st_disk_space->rrdlabels, "rw_mode", d->readonly ? "ro" : "rw", RRDLABEL_SRC_AUTO);

        {
            char buf[UINT64_HEX_MAX_LENGTH];
            print_uint64_hex(buf, d->SerialNumber);
            rrdlabels_add(d->st_disk_space->rrdlabels, "serial_number", buf, RRDLABEL_SRC_AUTO);
        }

        d->rd_disk_space_free = rrddim_add(d->st_disk_space, "avail", NULL, 1, d->divisor, RRD_ALGORITHM_ABSOLUTE);
        d->rd_disk_space_used = rrddim_add(d->st_disk_space, "used", NULL, 1, d->divisor, RRD_ALGORITHM_ABSOLUTE);
        d->chart_divisor = d->divisor;
    }
    else if (d->chart_divisor != d->divisor) {
        // the source switched between the Win32 APIs (bytes) and the perflib counter (MiB)
        rrddim_set_divisor(d->st_disk_space, d->rd_disk_space_free, (int32_t)d->divisor);
        rrddim_set_divisor(d->st_disk_space, d->rd_disk_space_used, (int32_t)d->divisor);
        d->chart_divisor = d->divisor;
    }

    // the perflib counter reports free space that can exceed the reported size on some volumes
    ULONGLONG free_space = d->percentDiskFree.current.Data;
    ULONGLONG total_space = (ULONGLONG)d->percentDiskFree.current.Time;
    ULONGLONG used_space = (total_space > free_space) ? total_space - free_space : 0;

    rrddim_set_by_pointer(d->st_disk_space, d->rd_disk_space_free, (collected_number)free_space);
    rrddim_set_by_pointer(d->st_disk_space, d->rd_disk_space_used, (collected_number)used_space);
    rrdset_done(d->st_disk_space);
}

struct logical_disk_collection_ops {
    PERF_OBJECT_TYPE *(*find_object)(PERF_DATA_BLOCK *pDataBlock, const char *name);
    PERF_INSTANCE_DEFINITION *(*next_instance)(
        PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *lastInstance);
    BOOL (*get_instance_name)(
        PERF_DATA_BLOCK *pDataBlock,
        PERF_OBJECT_TYPE *pObjectType,
        PERF_INSTANCE_DEFINITION *pInstance,
        char *buffer,
        size_t bufferLen);
    void (*collect_instance)(
        PERF_DATA_BLOCK *pDataBlock,
        PERF_OBJECT_TYPE *pObjectType,
        PERF_INSTANCE_DEFINITION *pInstance,
        const char *name,
        int update_every,
        usec_t now_ut);
};

static void logical_disk_collect_instance(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    const char *name,
    int update_every,
    usec_t now_ut)
{
    DICTIONARY *dict = logicalDisks;

    char resolved_name[ND_MOUNT_PATH_MAX];
    logical_disk_resolve_name(name, resolved_name, sizeof(resolved_name));
    if (logical_disk_is_excluded(resolved_name))
        return;

    struct logical_disk *d = dictionary_set(dict, resolved_name, NULL, sizeof(*d));
    d->last_collected = now_ut;

    logical_disk_collect_metadata(d, resolved_name, now_ut);

    logical_disk_set_space(pDataBlock, pObjectType, pi, d, resolved_name);

    // chart creation belongs to logical_disk_chart(), so both producers emit the same chart
    logical_disk_chart(d, resolved_name, update_every);
}

static bool do_logical_disk_with_ops(
    PERF_DATA_BLOCK *pDataBlock,
    int update_every,
    usec_t now_ut,
    const struct logical_disk_collection_ops *ops,
    char *instance_name,
    size_t instance_name_size)
{
    PERF_OBJECT_TYPE *pObjectType = ops->find_object(pDataBlock, "LogicalDisk");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = ops->next_instance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!ops->get_instance_name(pDataBlock, pObjectType, pi, instance_name, instance_name_size))
            strncpyz(instance_name, "[unknown]", instance_name_size - 1);

        if (strcasecmp(instance_name, "_Total") == 0 || logical_disk_is_excluded(instance_name))
            continue;

        ops->collect_instance(pDataBlock, pObjectType, pi, instance_name, update_every, now_ut);
    }

    return true;
}

// Mount-point dictionaries store NULL values, so dictionary_get() cannot distinguish a present
// path from an absent one.
static bool mount_points_contains(DICTIONARY *paths, const char *name)
{
    bool found = false;
    void *value;

    dfe_start_read(paths, value)
    {
        if (strcmp(v_dfe.name, name) == 0) {
            found = true;
            break;
        }
    }
    dfe_done(value);
    return found;
}

static bool logical_disk_collected_this_cycle(const struct logical_disk *d, usec_t now_ut)
{
    return d && d->last_collected == now_ut;
}

static bool volume_space_results_complete(DICTIONARY *results)
{
    if (!results)
        return false;

    bool complete = true;
    struct volume_space_result *result;
    dfe_start_read(results, result)
    {
        if (!result->success)
            complete = false;
    }
    dfe_done(result);
    return complete;
}

// Volumes perflib does not list: Cluster Shared Volumes, and any volume mounted into a folder that
// perflib skipped. Runs after the perflib pass, so volumes already collected this cycle are left
// alone and never produce a second chart. Space queries are completed by the slow worker.
static bool do_mount_points(int update_every, usec_t now_ut)
{
    DICTIONARY *results = volume_space_collect_results();
    if (!results)
        return false;

    bool collected = volume_space_results_complete(results);
    struct volume_space_result *result;
    dfe_start_read(results, result)
    {
        const char *name = v_dfe.name;

        if (!result->success)
            continue;

        if (logical_disk_is_excluded(name) || !mount_points_contains(mountPoints, name))
            continue;

        struct logical_disk *d = dictionary_get(logicalDisks, name);
        if (logical_disk_collected_this_cycle(d, now_ut))
            continue;

        d = dictionary_set(logicalDisks, name, NULL, sizeof(*d));

        logical_disk_collect_metadata(d, name, now_ut);

        d->divisor = GIGA_FACTOR;
        d->percentDiskFree.current.Data = (ULONGLONG)result->free_bytes;
        d->percentDiskFree.current.Time = (LONGLONG)result->total_bytes;
        d->last_collected = now_ut;

        logical_disk_chart(d, name, update_every);
    }
    dfe_done(result);
    dictionary_destroy(results);
    return collected;
}

// Evict only after the mount-point producer completed successfully. It covers every path-backed
// volume, while a failed slow filesystem pass leaves the last known instances intact.
static void logical_disk_evict_stale(usec_t now_ut)
{
    struct logical_disk *d;
    dfe_start_write(logicalDisks, d)
    {
        if (d->last_collected < now_ut)
            dictionary_del(logicalDisks, d_dfe.name);
    }
    dfe_done(d);
    dictionary_garbage_collect(logicalDisks);
}

static const struct logical_disk_collection_ops logical_disk_production_ops = {
    .find_object = perflibFindObjectTypeByName,
    .next_instance = perflibForEachInstance,
    .get_instance_name = getInstanceName,
    .collect_instance = logical_disk_collect_instance,
};

static bool do_logical_disk(PERF_DATA_BLOCK *pDataBlock, int update_every, usec_t now_ut)
{
    bool collected = do_logical_disk_with_ops(
        pDataBlock,
        update_every,
        now_ut,
        &logical_disk_production_ops,
        windows_shared_buffer,
        sizeof(windows_shared_buffer));

    // eviction is deliberately not done here - see logical_disk_evict_stale()
    return collected;
}

struct logical_disk_unittest_fixture {
    PERF_OBJECT_TYPE object;
    PERF_INSTANCE_DEFINITION instances[3];
    const char *names[3];
    const char *requested_object;
    char collected[3][128];
    size_t collected_count;
};

static struct logical_disk_unittest_fixture *logical_disk_unittest_fixture = NULL;

static PERF_OBJECT_TYPE *logical_disk_unittest_find_object(PERF_DATA_BLOCK *pDataBlock __maybe_unused, const char *name)
{
    logical_disk_unittest_fixture->requested_object = name;
    return strcmp(name, "LogicalDisk") == 0 ? &logical_disk_unittest_fixture->object : NULL;
}

static PERF_INSTANCE_DEFINITION *logical_disk_unittest_next_instance(
    PERF_DATA_BLOCK *pDataBlock __maybe_unused,
    PERF_OBJECT_TYPE *pObjectType __maybe_unused,
    PERF_INSTANCE_DEFINITION *lastInstance)
{
    size_t index = lastInstance ? (size_t)(lastInstance - logical_disk_unittest_fixture->instances) + 1 : 0;
    return index < (size_t)logical_disk_unittest_fixture->object.NumInstances ?
               &logical_disk_unittest_fixture->instances[index] :
               NULL;
}

static BOOL logical_disk_unittest_get_instance_name(
    PERF_DATA_BLOCK *pDataBlock __maybe_unused,
    PERF_OBJECT_TYPE *pObjectType __maybe_unused,
    PERF_INSTANCE_DEFINITION *pInstance,
    char *buffer,
    size_t bufferLen)
{
    size_t index = (size_t)(pInstance - logical_disk_unittest_fixture->instances);
    strncpyz(buffer, logical_disk_unittest_fixture->names[index], bufferLen - 1);
    return TRUE;
}

static void logical_disk_unittest_collect_instance(
    PERF_DATA_BLOCK *pDataBlock __maybe_unused,
    PERF_OBJECT_TYPE *pObjectType __maybe_unused,
    PERF_INSTANCE_DEFINITION *pInstance __maybe_unused,
    const char *name,
    int update_every __maybe_unused,
    usec_t now_ut __maybe_unused)
{
    size_t index = logical_disk_unittest_fixture->collected_count++;
    strncpyz(
        logical_disk_unittest_fixture->collected[index],
        name,
        sizeof(logical_disk_unittest_fixture->collected[index]) - 1);
}

static const struct logical_disk_collection_ops logical_disk_unittest_ops = {
    .find_object = logical_disk_unittest_find_object,
    .next_instance = logical_disk_unittest_next_instance,
    .get_instance_name = logical_disk_unittest_get_instance_name,
    .collect_instance = logical_disk_unittest_collect_instance,
};

static int logical_disk_unittest_run(
    const char *pattern,
    size_t expected_collected,
    const char *expected_first,
    const char *expected_second)
{
    struct logical_disk_unittest_fixture fixture = {
        .object = {.NumInstances = 3},
        .names = {"_Total", "C:", "Z:\\ASSUREDRECOVERYTEMP\\volume"},
    };
    char instance_name[128];
    SIMPLE_PATTERN *previous_pattern = excluded_logical_disk_paths;
    int errors = 0;

    excluded_logical_disk_paths = simple_pattern_create(pattern, NULL, SIMPLE_PATTERN_EXACT, false);
    logical_disk_unittest_fixture = &fixture;

    if (!do_logical_disk_with_ops(
            (PERF_DATA_BLOCK *)&fixture,
            1,
            1,
            &logical_disk_unittest_ops,
            instance_name,
            sizeof(instance_name))) {
        fprintf(stderr, "perflib storage unittest: failed to find LogicalDisk\n");
        errors++;
    }

    if (!fixture.requested_object || strcmp(fixture.requested_object, "LogicalDisk") != 0) {
        fprintf(stderr, "perflib storage unittest: queried the wrong Perflib object\n");
        errors++;
    }

    if (fixture.collected_count != expected_collected ||
        (expected_first && (!fixture.collected_count || strcmp(fixture.collected[0], expected_first) != 0)) ||
        (expected_second && (fixture.collected_count < 2 || strcmp(fixture.collected[1], expected_second) != 0))) {
        fprintf(stderr,
                "perflib storage unittest: expected %zu collected instances, got %zu\n",
                expected_collected,
                fixture.collected_count);
        errors++;
    }

    logical_disk_unittest_fixture = NULL;
    simple_pattern_free(excluded_logical_disk_paths);
    excluded_logical_disk_paths = previous_pattern;
    return errors;
}

// The scan stubs deliberately populate before failing: a real enumeration can add several volumes
// and then fail part way, and a partial snapshot must be discarded rather than swapped in.
static unsigned mount_points_unittest_volume_scans;
static unsigned mount_points_unittest_csv_scans;

static bool mount_points_unittest_scan_volumes_ok(
    DICTIONARY *paths, DICTIONARY *devices, DICTIONARY *excluded_devices __maybe_unused)
{
    mount_points_unittest_volume_scans++;
    dictionary_set(paths, "C:", NULL, 0);
    dictionary_set(devices, "HarddiskVolume7", (void *)"C:", sizeof("C:"));
    return true;
}

static bool mount_points_unittest_scan_volumes_fail(
    DICTIONARY *paths, DICTIONARY *devices, DICTIONARY *excluded_devices __maybe_unused)
{
    mount_points_unittest_volume_scans++;
    // two paths, so a leaked partial snapshot also differs from the retained one by entry count
    dictionary_set(paths, "E:", NULL, 0);
    dictionary_set(paths, "F:", NULL, 0);
    dictionary_set(devices, "HarddiskVolume9", (void *)"E:", sizeof("E:"));
    return false;
}

static bool mount_points_unittest_scan_csv_ok(DICTIONARY *paths)
{
    mount_points_unittest_csv_scans++;
    dictionary_set(paths, "C:\\ClusterStorage\\Volume1", NULL, 0);
    return true;
}

static bool mount_points_unittest_scan_csv_fail(DICTIONARY *paths)
{
    mount_points_unittest_csv_scans++;
    dictionary_set(paths, "C:\\ClusterStorage\\Volume2", NULL, 0);
    return false;
}

static const struct mount_points_scan_ops mount_points_unittest_ops_ok = {
    .scan_volumes = mount_points_unittest_scan_volumes_ok,
    .scan_cluster_storage = mount_points_unittest_scan_csv_ok,
};

static const struct mount_points_scan_ops mount_points_unittest_ops_volumes_fail = {
    .scan_volumes = mount_points_unittest_scan_volumes_fail,
    .scan_cluster_storage = mount_points_unittest_scan_csv_ok,
};

static const struct mount_points_scan_ops mount_points_unittest_ops_csv_fail = {
    .scan_volumes = mount_points_unittest_scan_volumes_ok,
    .scan_cluster_storage = mount_points_unittest_scan_csv_fail,
};

static bool mount_points_unittest_has(DICTIONARY *paths, const char *name)
{
    return mount_points_contains(paths, name);
}

static int mount_points_unittest_expect(const char *what, bool condition)
{
    if (condition)
        return 0;

    fprintf(stderr, "perflib storage unittest: %s\n", what);
    return 1;
}

static bool volume_space_unittest_ok(const char *name __maybe_unused, uint64_t *total, uint64_t *free)
{
    *total = UINT64_C(8) * GIGA_FACTOR;
    *free = UINT64_C(3) * GIGA_FACTOR;
    return true;
}

static int storage_helpers_unittest_run(void)
{
    int errors = 0;
    char path[128];

    strncpyz(path, "C:\\\\", sizeof(path) - 1);
    canonicalize_mount_path(path, sizeof(path));
    errors += mount_points_unittest_expect("mount paths are canonicalized", strcmp(path, "C:") == 0);

    char selected[128] = "C:\\mount";
    mount_points_choose_path(selected, sizeof(selected), "D:");
    errors += mount_points_unittest_expect("drive letters are preferred", strcmp(selected, "D:") == 0);
    mount_points_choose_path(selected, sizeof(selected), "C:");
    errors += mount_points_unittest_expect("drive-letter choice is deterministic", strcmp(selected, "C:") == 0);

    DICTIONARY *previous_devices = deviceMountPaths;
    deviceMountPaths = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_set(deviceMountPaths, "HarddiskVolume9", (void *)"E:", sizeof("E:"));
    char resolved[128];
    logical_disk_resolve_name("HarddiskVolume9", resolved, sizeof(resolved));
    errors += mount_points_unittest_expect("device names resolve to mount paths", strcmp(resolved, "E:") == 0);
    dictionary_destroy(deviceMountPaths);
    deviceMountPaths = previous_devices;

    DICTIONARY *previous_excluded_devices = excludedDeviceNames;
    excludedDeviceNames = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_set(excludedDeviceNames, "HarddiskVolume10", "HarddiskVolume10", sizeof("HarddiskVolume10"));
    errors += mount_points_unittest_expect(
        "fully excluded devices stay excluded by their bare name", logical_disk_is_excluded("HarddiskVolume10"));
    dictionary_destroy(excludedDeviceNames);
    excludedDeviceNames = previous_excluded_devices;

    wchar_t root[128];
    char root_utf8[128];
    bool truncated = false;
    errors += mount_points_unittest_expect(
        "drive roots convert to complete wide paths", volume_root_path_w("C:", root, _countof(root)));
    if (utf16_to_utf8(root_utf8, sizeof(root_utf8), root, -1, &truncated) > 0) {
        errors += mount_points_unittest_expect("drive root conversion is not truncated", !truncated);
        errors += mount_points_unittest_expect("drive roots receive a trailing separator", strcmp(root_utf8, "C:\\") == 0);
    }

    struct logical_disk d = { .last_collected = 42 };
    errors += mount_points_unittest_expect("duplicate chart guard recognizes current samples",
                                            logical_disk_collected_this_cycle(&d, 42));
    errors += mount_points_unittest_expect("duplicate chart guard rejects old samples",
                                            !logical_disk_collected_this_cycle(&d, 43));

    memset(&d, 0, sizeof(d));
    logical_disk_set_space_with_ops(NULL, NULL, NULL, &d, "C:", volume_space_unittest_ok);
    errors += mount_points_unittest_expect("Win32 byte values use the GiB divisor", d.divisor == GIGA_FACTOR);
    errors += mount_points_unittest_expect("Win32 byte values preserve free bytes",
                                            d.percentDiskFree.current.Data == UINT64_C(3) * GIGA_FACTOR);

    return errors;
}

// A transient enumeration failure must not empty the registry: do_mount_points() would stop
// publishing the CSV instances, logical_disk_evict_stale() would obsolete their charts, and
// logical_disk_resolve_name() would fall back to bare device names - all of it recovering only a
// refresh interval later.
static int mount_points_unittest_run(void)
{
    DICTIONARY *previous_paths = mountPoints;
    DICTIONARY *previous_devices = deviceMountPaths;
    DICTIONARY *previous_excluded_devices = excludedDeviceNames;
    usec_t previous_refresh_ut = mountPointsRefreshedUT;
    usec_t now_ut = MOUNT_POINTS_REFRESH_EVERY_UT;
    int errors = 0;

    mountPoints = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    deviceMountPaths = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    excludedDeviceNames = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    mountPointsRefreshedUT = 0;
    mount_points_unittest_volume_scans = 0;
    mount_points_unittest_csv_scans = 0;

    mount_points_refresh_with_ops(now_ut, &mount_points_unittest_ops_ok);
    errors += mount_points_unittest_expect(
        "a healthy scan must populate the registry", dictionary_entries(mountPoints) == 2);
    errors += mount_points_unittest_expect(
        "a healthy scan must map device names", dictionary_get(deviceMountPaths, "HarddiskVolume7") != NULL);

    now_ut += MOUNT_POINTS_REFRESH_EVERY_UT;
    mount_points_refresh_with_ops(now_ut, &mount_points_unittest_ops_volumes_fail);
    errors += mount_points_unittest_expect(
        "a failed volume scan must retain the previous registry", dictionary_entries(mountPoints) == 2);
    errors += mount_points_unittest_expect(
        "a failed volume scan must not leak its partial results", !mount_points_unittest_has(mountPoints, "E:"));
    errors += mount_points_unittest_expect(
        "a failed volume scan must retain the device map",
        dictionary_get(deviceMountPaths, "HarddiskVolume7") != NULL);

    now_ut += MOUNT_POINTS_REFRESH_EVERY_UT;
    mount_points_refresh_with_ops(now_ut, &mount_points_unittest_ops_csv_fail);
    errors += mount_points_unittest_expect(
        "a failed ClusterStorage scan must retain the previous registry",
        mount_points_unittest_has(mountPoints, "C:\\ClusterStorage\\Volume1"));
    errors += mount_points_unittest_expect(
        "a failed ClusterStorage scan must not leak its partial results",
        !mount_points_unittest_has(mountPoints, "C:\\ClusterStorage\\Volume2"));

    // a refresh inside the interval must not rescan at all, so an armed failure cannot be observed
    unsigned volume_scans_before_interval = mount_points_unittest_volume_scans;
    unsigned csv_scans_before_interval = mount_points_unittest_csv_scans;
    mount_points_refresh_with_ops(now_ut + 1, &mount_points_unittest_ops_volumes_fail);
    errors += mount_points_unittest_expect(
        "no volume rescan inside the refresh interval",
        mount_points_unittest_volume_scans == volume_scans_before_interval);
    errors += mount_points_unittest_expect(
        "no CSV rescan inside the refresh interval", mount_points_unittest_csv_scans == csv_scans_before_interval);

    unsigned volume_scans_before_recovery = mount_points_unittest_volume_scans;
    unsigned csv_scans_before_recovery = mount_points_unittest_csv_scans;
    now_ut += MOUNT_POINTS_REFRESH_EVERY_UT;
    mount_points_refresh_with_ops(now_ut, &mount_points_unittest_ops_ok);
    errors += mount_points_unittest_expect(
        "the registry must still refresh after a failure",
        dictionary_entries(mountPoints) == 2 &&
            mount_points_unittest_volume_scans == volume_scans_before_recovery + 1 &&
            mount_points_unittest_csv_scans == csv_scans_before_recovery + 1);

    dictionary_destroy(mountPoints);
    dictionary_destroy(deviceMountPaths);
    mountPoints = previous_paths;
    deviceMountPaths = previous_devices;
    dictionary_destroy(excludedDeviceNames);
    excludedDeviceNames = previous_excluded_devices;
    mountPointsRefreshedUT = previous_refresh_ut;
    return errors;
}

static int mount_points_query_failure_unittest_run(void)
{
    DICTIONARY *previous_paths = mountPoints;
    DICTIONARY *previous_results = volume_space_results;
    DICTIONARY *previous_disks = logicalDisks;
    int errors = 0;

    mountPoints = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_set(mountPoints, "C:\\ClusterStorage\\Volume1", NULL, 0);
    volume_space_results = dictionary_create_advanced(
        DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct volume_space_result));
    struct volume_space_result failed = { .success = false };
    dictionary_set(volume_space_results, "C:\\ClusterStorage\\Volume1", &failed, sizeof(failed));
    logicalDisks = dictionary_create(DICT_OPTION_SINGLE_THREADED);

    errors += mount_points_unittest_expect(
        "a failed volume-space result makes mount-point collection fail",
        !volume_space_results_complete(volume_space_results));

    dictionary_destroy(mountPoints);
    dictionary_destroy(logicalDisks);
    mountPoints = previous_paths;
    logicalDisks = previous_disks;
    dictionary_destroy(volume_space_results);
    volume_space_results = previous_results;
    return errors;
}

int perflib_storage_unittest(void)
{
    int errors = 0;

    errors += storage_helpers_unittest_run();
    errors += logical_disk_unittest_run("*AssuredRecoveryTemp*", 1, "C:", NULL);
    errors += logical_disk_unittest_run(NULL, 2, "C:", "Z:\\ASSUREDRECOVERYTEMP\\volume");
    errors += mount_points_query_failure_unittest_run();
    errors += mount_points_unittest_run();

    if (errors)
        fprintf(stderr, "perflib storage unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "perflib storage unittest: OK\n");

    return errors;
}

static void physical_disk_labels(RRDSET *st, void *data)
{
    struct physical_disk *d = data;

    if (d->device)
        rrdlabels_add(st->rrdlabels, "device", string2str(d->device), RRDLABEL_SRC_AUTO);

    if (d->mount_point)
        rrdlabels_add(st->rrdlabels, "mount_point", string2str(d->mount_point), RRDLABEL_SRC_AUTO);

    //    if (d->manufacturer)
    //        rrdlabels_add(st->rrdlabels, "manufacturer", string2str(d->manufacturer), RRDLABEL_SRC_AUTO);

    if (d->model)
        rrdlabels_add(st->rrdlabels, "model", string2str(d->model), RRDLABEL_SRC_AUTO);

    //    if (d->media_type)
    //        rrdlabels_add(st->rrdlabels, "media_type", string2str(d->media_type), RRDLABEL_SRC_AUTO);

    //    if (d->name)
    //        rrdlabels_add(st->rrdlabels, "name", string2str(d->name), RRDLABEL_SRC_AUTO);

    if (d->device_id)
        rrdlabels_add(st->rrdlabels, "device_id", string2str(d->device_id), RRDLABEL_SRC_AUTO);
}

static bool str_is_numeric(const char *s)
{
    while (*s)
        if (!isdigit((uint8_t)*s++))
            return false;
    return true;
}

static inline double perflib_average_timer_ms(COUNTER_DATA *d)
{
    if (!d->updated)
        return 0.0;

    ULONGLONG data1 = d->current.Data;
    ULONGLONG data0 = d->previous.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG time0 = d->previous.Time;
    LONGLONG freq1 = d->current.Frequency;

    if (data1 >= data0 && time1 > time0 && time0 && freq1)
        return ((double)(data1 - data0) * (double)MSEC_PER_SEC) / ((double)freq1 * (double)(time1 - time0));

    return 0;
}

static inline uint64_t perflib_average_bulk(COUNTER_DATA *d)
{
    if (!d->updated)
        return 0;

    ULONGLONG data1 = d->current.Data;
    ULONGLONG data0 = d->previous.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG time0 = d->previous.Time;

    if (data1 >= data0 && time1 > time0 && time0)
        return (data1 - data0) / (time1 - time0);

    return 0;
}

static inline uint64_t perflib_idle_time_percent(COUNTER_DATA *d)
{
    if (!d->updated)
        return 0.0;

    ULONGLONG data1 = d->current.Data;
    ULONGLONG data0 = d->previous.Data;
    LONGLONG time1 = d->current.Time;
    LONGLONG time0 = d->previous.Time;

    if (data1 >= data0 && time1 > time0 && time0) {
        uint64_t pcent = 100 * (data1 - data0) / (time1 - time0);
        return pcent > 100 ? 100 : pcent;
    }

    return 0;
}

#define MAX_WMI_DRIVES 100
static DiskDriveInfoWMI infos[MAX_WMI_DRIVES];

static bool do_physical_disk(PERF_DATA_BLOCK *pDataBlock, int update_every, usec_t now_ut)
{
    DICTIONARY *dict = physicalDisks;
    size_t infoCount = 0;
    bool infos_loaded = false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "PhysicalDisk");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        int device_index = -1;
        char *device = windows_shared_buffer;
        char mount_point[128];
        mount_point[0] = '\0';

        struct physical_disk *d;
        bool is_system;
        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            d = &system_physical_total;
            is_system = true;
        } else {
            char *space;
            if ((space = strchr(windows_shared_buffer, ' '))) {
                *space++ = '\0';
                strncpyz(mount_point, space, sizeof(mount_point) - 1);
            }

            if (str_is_numeric(windows_shared_buffer)) {
                device_index = str2ull(device, NULL);
                snprintfz(windows_shared_buffer, sizeof(windows_shared_buffer), "Disk %d", device_index);
                device = windows_shared_buffer;
            }

            d = dictionary_set(dict, device, NULL, sizeof(*d));
            is_system = false;
        }
        d->last_collected = now_ut;

        if (!d->collected_metadata) {
            if (!is_system && device_index != -1) {
                if (!infos_loaded) {
                    infoCount = GetDiskDriveInfo(infos, _countof(infos));
                    infos_loaded = true;
                }

                for (size_t k = 0; k < infoCount; k++) {
                    if (infos[k].Index != device_index)
                        continue;

                    d->manufacturer = string_strdupz(infos[k].Manufacturer);
                    d->model = string_strdupz(infos[k].Model);
                    d->media_type = string_strdupz(infos[k].MediaType);
                    d->name = string_strdupz(infos[k].Name);
                    d->device_id = string_strdupz(infos[k].DeviceID);

                    break;
                }
            }

            d->device = string_strdupz(device);
            d->mount_point = string_strdupz(mount_point);
            d->collected_metadata = true;
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskReadBytesPerSec) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskWriteBytesPerSec)) {
            if (is_system)
                common_system_io(
                    d->diskReadBytesPerSec.current.Data, d->diskWriteBytesPerSec.current.Data, update_every);
            else
                common_disk_io(
                    &d->disk_io,
                    device,
                    NULL,
                    d->diskReadBytesPerSec.current.Data,
                    d->diskWriteBytesPerSec.current.Data,
                    update_every,
                    physical_disk_labels,
                    d);
        }

        if (is_system)
            continue;

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskReadsPerSec) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskWritesPerSec)) {
            common_disk_ops(
                &d->disk_ops,
                device,
                NULL,
                d->diskReadsPerSec.current.Data,
                d->diskWritesPerSec.current.Data,
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentIdleTime)) {
            common_disk_util(
                &d->disk_util,
                device,
                NULL,
                100 - perflib_idle_time_percent(&d->percentIdleTime),
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskTime)) {
            common_disk_busy(
                &d->disk_busy,
                device,
                NULL,
                d->percentDiskTime.current.Data / NS100_PER_MS,
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskReadTime) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskWriteTime)) {
            common_disk_iotime(
                &d->disk_iotime,
                device,
                NULL,
                d->percentDiskReadTime.current.Data / NS100_PER_MS,
                d->percentDiskWriteTime.current.Data / NS100_PER_MS,
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->currentDiskQueueLength)) {
            common_disk_qops(
                &d->disk_qops,
                device,
                NULL,
                d->currentDiskQueueLength.current.Data,
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerRead) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerWrite)) {
            common_disk_await(
                &d->disk_await,
                device,
                NULL,
                perflib_average_timer_ms(&d->averageDiskSecondsPerRead),
                perflib_average_timer_ms(&d->averageDiskSecondsPerWrite),
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerTransfer)) {
            common_disk_svctm(
                &d->disk_svctm,
                device,
                NULL,
                perflib_average_timer_ms(&d->averageDiskSecondsPerTransfer),
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskBytesPerRead) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskBytesPerWrite)) {
            common_disk_avgsz(
                &d->disk_avgsz,
                device,
                NULL,
                perflib_average_bulk(&d->averageDiskBytesPerRead),
                perflib_average_bulk(&d->averageDiskBytesPerWrite),
                update_every,
                physical_disk_labels,
                d);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->splitIoPerSec)) {
            if (!d->st_split) {
                d->st_split = rrdset_create_localhost(
                    "disk_split",
                    device,
                    NULL,
                    "iops",
                    "disk.split",
                    "Split I/O Operations",
                    "operations/s",
                    _COMMON_PLUGIN_NAME,
                    _COMMON_PLUGIN_MODULE_NAME,
                    NETDATA_CHART_PRIO_DISK_SPLIT,
                    update_every,
                    RRDSET_TYPE_LINE);

                d->rd_split = rrddim_add(d->st_split, "discards", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                physical_disk_labels(d->st_split, d);
            }

            rrddim_set_by_pointer(d->st_split, d->rd_split, d->splitIoPerSec.current.Data);
            rrdset_done(d->st_split);
        }
    }

    // cleanup - delete callback will handle resource cleanup
    {
        struct physical_disk *d;
        dfe_start_write(dict, d)
        {
            if (d->last_collected < now_ut)
                dictionary_del(dict, d_dfe.name);
        }
        dfe_done(d);
        dictionary_garbage_collect(dict);
    }

    return true;
}

int do_PerflibStorage(int update_every, usec_t dt __maybe_unused)
{
    DWORD logical_id, physical_id;

    if (unlikely(!storage_initialized)) {
        initialize();
    }

    logical_id = RegistryFindIDByName("LogicalDisk");
    physical_id = RegistryFindIDByName("PhysicalDisk");

    // Each perflibGetPerformanceData() call reuses the same internal buffer, so we must
    // query and consume each block before issuing the next call to avoid pointer aliasing.
    usec_t now_ut = now_monotonic_usec();

    // must happen before the perflib pass: it resolves perflib device names to mount paths
    mount_points_refresh(now_ut);

    // A missing registry object is a transient collection failure; returning success keeps this
    // module enabled while the next cycle retries the query.
    if (logical_id != PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(logical_id);
        if (pDataBlock)
            do_logical_disk(pDataBlock, update_every, now_ut);
    }

    volume_space_submit();
    bool mount_points_collected = do_mount_points(update_every, now_ut);
    // The mount-point producer covers every path-backed volume, including systems without a
    // LogicalDisk PerfLib object. Its complete result is therefore sufficient for eviction.
    if (mount_points_collected)
        logical_disk_evict_stale(now_ut);

    if (physical_id != PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(physical_id);
        if (pDataBlock)
            do_physical_disk(pDataBlock, update_every, now_ut);
    }

    return 0;
}

void do_PerflibStorage_cleanup(void)
{
    if (!storage_initialized)
        return;

    netdata_mutex_lock(&volume_space_mutex);
    volume_space_worker_stop = true;
    netdata_cond_broadcast(&volume_space_cond);
    netdata_mutex_unlock(&volume_space_mutex);

    if (volume_space_thread) {
        nd_thread_join(volume_space_thread);
        volume_space_thread = NULL;
    }

    netdata_mutex_lock(&volume_space_mutex);
    dictionary_destroy(volume_space_request);
    volume_space_request = NULL;
    dictionary_destroy(volume_space_results);
    volume_space_results = NULL;
    netdata_mutex_unlock(&volume_space_mutex);

    netdata_cond_destroy(&volume_space_cond);
    netdata_mutex_destroy(&volume_space_mutex);

    dictionary_destroy(logicalDisks);
    logicalDisks = NULL;
    dictionary_destroy(physicalDisks);
    physicalDisks = NULL;
    dictionary_destroy(mountPoints);
    mountPoints = NULL;
    dictionary_destroy(deviceMountPaths);
    deviceMountPaths = NULL;
    dictionary_destroy(excludedDeviceNames);
    excludedDeviceNames = NULL;
    simple_pattern_free(excluded_logical_disk_paths);
    excluded_logical_disk_paths = NULL;
    storage_initialized = false;
}
