// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_macos.h"

#include <CoreFoundation/CoreFoundation.h>
#include <math.h>
#include <poll.h>
#include <strings.h>
#include <unistd.h>

#define MACOS_POWERMETRICS_DEFAULT_COMMAND "/usr/bin/powermetrics"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_GPU "powermetrics-thermal-smc-gpu"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_GPU "powermetrics-thermal-gpu"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC "powermetrics-thermal-smc"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL "powermetrics-thermal"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_GPU_LOOP "powermetrics-thermal-smc-gpu-loop"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_GPU_LOOP "powermetrics-thermal-gpu-loop"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_LOOP "powermetrics-thermal-smc-loop"
#define MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_LOOP "powermetrics-thermal-loop"
#define MACOS_POWERMETRICS_SAMPLERS_THERMAL_SMC_GPU "thermal,smc,gpu_power"
#define MACOS_POWERMETRICS_SAMPLERS_THERMAL_GPU "thermal,gpu_power"
#define MACOS_POWERMETRICS_SAMPLERS_THERMAL_SMC "thermal,smc"
#define MACOS_POWERMETRICS_SAMPLERS_THERMAL "thermal"
#define MACOS_POWERMETRICS_DEFAULT_SAMPLE_EVERY 60
#define MACOS_POWERMETRICS_DEFAULT_SAMPLE_WINDOW_MS 1000
// Must match the ceiling the setuid ndsudo helper enforces for powermetrics
// interval arguments; configured values are clamped so ndsudo never rejects them.
#define MACOS_POWERMETRICS_INTERVAL_MS_MAX 60000
#define MACOS_POWERMETRICS_DEFAULT_TIMEOUT_MS 5000
#define MACOS_POWERMETRICS_READ_STEP_MS 250
#define MACOS_POWERMETRICS_MAX_OUTPUT (1024 * 1024)
#define MACOS_POWERMETRICS_INITIAL_BACKOFF_MS 1000
#define MACOS_POWERMETRICS_MAX_BACKOFF_MS 60000
#define MACOS_POWERMETRICS_STARTUP_FAILURES_MAX 3

enum macos_thermal_pressure {
    MACOS_THERMAL_PRESSURE_NOMINAL,
    MACOS_THERMAL_PRESSURE_MODERATE,
    MACOS_THERMAL_PRESSURE_HEAVY,
    MACOS_THERMAL_PRESSURE_SLEEPING,
    MACOS_THERMAL_PRESSURE_TRAPPING,
    MACOS_THERMAL_PRESSURE_UNDEFINED,
    MACOS_THERMAL_PRESSURE_COUNT,
};

enum macos_powermetrics_sampler {
    MACOS_POWERMETRICS_SAMPLER_NONE,
    MACOS_POWERMETRICS_SAMPLER_THERMAL_SMC_GPU,
    MACOS_POWERMETRICS_SAMPLER_THERMAL_GPU,
    MACOS_POWERMETRICS_SAMPLER_THERMAL_SMC,
    MACOS_POWERMETRICS_SAMPLER_THERMAL,
};

struct macos_powermetrics_sampler_spec {
    enum macos_powermetrics_sampler id;
    const char *samplers;
    const char *ndsudo_probe_command;
    const char *ndsudo_loop_command;
};

struct macos_powermetrics_sample {
    bool valid;
    bool has_thermal_pressure;
    enum macos_thermal_pressure thermal_pressure;

    bool has_fan;
    double fan_rpm;

    bool has_cpu_die;
    double cpu_die_c;
    bool has_gpu_die;
    double gpu_die_c;

    bool has_cpu_thermal_level;
    uint64_t cpu_thermal_level;
    bool has_gpu_thermal_level;
    uint64_t gpu_thermal_level;
    bool has_io_thermal_level;
    uint64_t io_thermal_level;

    bool has_cpu_prochot;
    bool cpu_prochot;
    bool has_smc_prochot;
    bool smc_prochot;

    bool has_gpu_power;
    double gpu_power_w;

    usec_t collected_ut;
};

struct macos_powermetrics_state {
    bool initialized;
    bool failed_permanently;
    bool logged_unavailable;
    bool logged_loop_failure;
    int consecutive_failures;

    bool use_ndsudo;
    int sample_interval_ms;
    int sample_window_ms;
    int command_timeout_ms;
    char command[FILENAME_MAX + 1];

    netdata_mutex_t mutex;
    ND_THREAD *thread;
    struct macos_powermetrics_sample sample;
    uint64_t sample_sequence;
};

static struct macos_powermetrics_state pm = {0};

static RRDSET *st_thermal_pressure = NULL;
static RRDDIM *rd_thermal_pressure[MACOS_THERMAL_PRESSURE_COUNT] = {0};
static RRDSET *st_smc_thermal_level = NULL;
static RRDDIM *rd_smc_cpu_thermal_level = NULL;
static RRDDIM *rd_smc_gpu_thermal_level = NULL;
static RRDDIM *rd_smc_io_thermal_level = NULL;
static RRDSET *st_smc_prochot = NULL;
static RRDDIM *rd_smc_cpu_prochot = NULL;
static RRDDIM *rd_smc_prochot = NULL;

static RRDSET *st_fan = NULL;
static RRDDIM *rd_fan = NULL;
static RRDSET *st_cpu_die = NULL;
static RRDDIM *rd_cpu_die = NULL;
static RRDSET *st_gpu_die = NULL;
static RRDDIM *rd_gpu_die = NULL;
static RRDSET *st_gpu_power = NULL;
static RRDDIM *rd_gpu_power = NULL;

static const struct macos_powermetrics_sampler_spec macos_powermetrics_sampler_specs[] = {
    {
        .id = MACOS_POWERMETRICS_SAMPLER_THERMAL_SMC_GPU,
        .samplers = MACOS_POWERMETRICS_SAMPLERS_THERMAL_SMC_GPU,
        .ndsudo_probe_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_GPU,
        .ndsudo_loop_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_GPU_LOOP,
    },
    {
        .id = MACOS_POWERMETRICS_SAMPLER_THERMAL_GPU,
        .samplers = MACOS_POWERMETRICS_SAMPLERS_THERMAL_GPU,
        .ndsudo_probe_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_GPU,
        .ndsudo_loop_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_GPU_LOOP,
    },
    {
        .id = MACOS_POWERMETRICS_SAMPLER_THERMAL_SMC,
        .samplers = MACOS_POWERMETRICS_SAMPLERS_THERMAL_SMC,
        .ndsudo_probe_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC,
        .ndsudo_loop_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_SMC_LOOP,
    },
    {
        .id = MACOS_POWERMETRICS_SAMPLER_THERMAL,
        .samplers = MACOS_POWERMETRICS_SAMPLERS_THERMAL,
        .ndsudo_probe_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL,
        .ndsudo_loop_command = MACOS_POWERMETRICS_NDSUDO_COMMAND_THERMAL_LOOP,
    },
};

static const char *macos_thermal_pressure_names[MACOS_THERMAL_PRESSURE_COUNT] = {
    [MACOS_THERMAL_PRESSURE_NOMINAL] = "nominal",
    [MACOS_THERMAL_PRESSURE_MODERATE] = "moderate",
    [MACOS_THERMAL_PRESSURE_HEAVY] = "heavy",
    [MACOS_THERMAL_PRESSURE_SLEEPING] = "sleeping",
    [MACOS_THERMAL_PRESSURE_TRAPPING] = "trapping",
    [MACOS_THERMAL_PRESSURE_UNDEFINED] = "undefined",
};

static void macos_powermetrics_mark_charts_obsolete(void)
{
    if (st_thermal_pressure)
        rrdset_is_obsolete___safe_from_collector_thread(st_thermal_pressure);
    if (st_smc_thermal_level)
        rrdset_is_obsolete___safe_from_collector_thread(st_smc_thermal_level);
    if (st_smc_prochot)
        rrdset_is_obsolete___safe_from_collector_thread(st_smc_prochot);
    if (st_fan)
        rrdset_is_obsolete___safe_from_collector_thread(st_fan);
    if (st_cpu_die)
        rrdset_is_obsolete___safe_from_collector_thread(st_cpu_die);
    if (st_gpu_die)
        rrdset_is_obsolete___safe_from_collector_thread(st_gpu_die);
    if (st_gpu_power)
        rrdset_is_obsolete___safe_from_collector_thread(st_gpu_power);
}

static bool cf_dictionary_get_double(CFDictionaryRef dict, CFStringRef key, double *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFNumberGetTypeID())
        return false;

    return CFNumberGetValue((CFNumberRef)obj, kCFNumberDoubleType, value);
}

static bool cf_dictionary_get_uint64(CFDictionaryRef dict, CFStringRef key, uint64_t *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFNumberGetTypeID())
        return false;

    int64_t signed_value = 0;
    if (!CFNumberGetValue((CFNumberRef)obj, kCFNumberSInt64Type, &signed_value) || signed_value < 0)
        return false;

    *value = (uint64_t)signed_value;
    return true;
}

static bool cf_dictionary_get_bool(CFDictionaryRef dict, CFStringRef key, bool *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFBooleanGetTypeID())
        return false;

    *value = CFBooleanGetValue((CFBooleanRef)obj);
    return true;
}

static bool cf_dictionary_get_nested_dictionary(CFDictionaryRef dict, CFStringRef key, CFDictionaryRef *value)
{
    if (!dict || !key || !value)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFDictionaryGetTypeID())
        return false;

    *value = (CFDictionaryRef)obj;
    return true;
}

static bool cf_dictionary_get_cstring(CFDictionaryRef dict, CFStringRef key, char *dst, size_t dst_size)
{
    if (!dict || !key || !dst || dst_size == 0)
        return false;

    CFTypeRef obj = CFDictionaryGetValue(dict, key);
    if (!obj || CFGetTypeID(obj) != CFStringGetTypeID())
        return false;

    if (!CFStringGetCString((CFStringRef)obj, dst, dst_size, kCFStringEncodingUTF8))
        return false;

    dst[dst_size - 1] = '\0';
    return dst[0] != '\0';
}

static bool macos_powermetrics_parse_gpu_power(CFDictionaryRef root, double *power_w)
{
    double power_mw = 0.0;
    if (cf_dictionary_get_double(root, CFSTR("gpu_power"), &power_mw)) {
        *power_w = power_mw / 1000.0;
        return true;
    }

    CFDictionaryRef gpu_dict = NULL;
    if (cf_dictionary_get_nested_dictionary(root, CFSTR("gpu"), &gpu_dict) &&
        cf_dictionary_get_double(gpu_dict, CFSTR("gpu_power"), &power_mw)) {
        *power_w = power_mw / 1000.0;
        return true;
    }

    return false;
}

static enum macos_thermal_pressure macos_thermal_pressure_from_string(const char *value)
{
    if (!value || !*value)
        return MACOS_THERMAL_PRESSURE_UNDEFINED;
    if (!strcasecmp(value, "Nominal"))
        return MACOS_THERMAL_PRESSURE_NOMINAL;
    if (!strcasecmp(value, "Moderate"))
        return MACOS_THERMAL_PRESSURE_MODERATE;
    if (!strcasecmp(value, "Heavy"))
        return MACOS_THERMAL_PRESSURE_HEAVY;
    if (!strcasecmp(value, "Sleeping"))
        return MACOS_THERMAL_PRESSURE_SLEEPING;
    if (!strcasecmp(value, "Trapping"))
        return MACOS_THERMAL_PRESSURE_TRAPPING;

    return MACOS_THERMAL_PRESSURE_UNDEFINED;
}

static bool macos_powermetrics_append(char **dst, size_t *used, size_t *size, const char *src, size_t len)
{
    if (*used + len > MACOS_POWERMETRICS_MAX_OUTPUT)
        return false;

    if (*used + len + 1 > *size) {
        size_t new_size = *size ? *size : 4096;
        while (new_size < *used + len + 1)
            new_size *= 2;

        if (new_size > MACOS_POWERMETRICS_MAX_OUTPUT + 1)
            new_size = MACOS_POWERMETRICS_MAX_OUTPUT + 1;

        *dst = reallocz(*dst, new_size);
        *size = new_size;
    }

    memcpy(*dst + *used, src, len);
    *used += len;
    (*dst)[*used] = '\0';

    return true;
}

static bool macos_powermetrics_read_stdout(POPEN_INSTANCE *pi, char **output, size_t *output_size, int timeout_ms)
{
    int fd = spawn_popen_read_fd(pi);
    if (fd < 0)
        return false;

    usec_t stop_ut = now_monotonic_usec() + (usec_t)timeout_ms * USEC_PER_MS;
    char *buf = NULL;
    size_t used = 0, size = 0;
    bool ok = true;

    while (!nd_thread_signaled_to_cancel()) {
        usec_t now_ut = now_monotonic_usec();
        if (now_ut >= stop_ut) {
            ok = false;
            break;
        }

        int remaining_ms = (int)((stop_ut - now_ut) / USEC_PER_MS);
        if (remaining_ms > MACOS_POWERMETRICS_READ_STEP_MS)
            remaining_ms = MACOS_POWERMETRICS_READ_STEP_MS;
        if (remaining_ms < 1)
            remaining_ms = 1;

        struct pollfd pfd = {
            .fd = fd,
            .events = POLLIN | POLLHUP,
        };

        int rc = poll(&pfd, 1, remaining_ms);
        if (rc < 0) {
            if (errno == EINTR)
                continue;
            ok = false;
            break;
        }

        if (rc == 0)
            continue;

        if (pfd.revents & POLLIN) {
            char tmp[4096];
            ssize_t bytes = read(fd, tmp, sizeof(tmp));
            if (bytes < 0) {
                if (errno == EINTR)
                    continue;
                ok = false;
                break;
            }
            if (bytes == 0)
                break;
            if (!macos_powermetrics_append(&buf, &used, &size, tmp, (size_t)bytes)) {
                ok = false;
                break;
            }
        }

        // Drain any buffered data before reacting to POLLHUP: poll reports
        // POLLIN | POLLHUP together when the writer closes with data still in the
        // pipe, and a single read() only pulls a chunk. Breaking on POLLHUP while
        // POLLIN is also set would discard the tail of the plist.
        if ((pfd.revents & POLLHUP) && !(pfd.revents & POLLIN))
            break;

        // POLLERR/POLLNVAL without POLLIN would neither read nor break, making the
        // loop spin until the step timeout; treat a pipe error as a hard failure.
        if (pfd.revents & (POLLERR | POLLNVAL)) {
            ok = false;
            break;
        }
    }

    if (nd_thread_signaled_to_cancel())
        ok = false;

    if (!ok) {
        freez(buf);
        return false;
    }

    *output = buf;
    *output_size = used;
    return true;
}

static bool macos_powermetrics_parse_plist(const char *data, size_t size, struct macos_powermetrics_sample *sample)
{
    if (!data || size == 0 || !sample)
        return false;

    while (size > 0 && data[size - 1] == '\0')
        size--;
    for (size_t i = 0; i < size; i++) {
        if (data[i] == '\0') {
            size = i;
            break;
        }
    }

    if (size == 0)
        return false;

    CFDataRef cf_data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)data, (CFIndex)size);
    if (!cf_data)
        return false;

    CFErrorRef error = NULL;
    CFPropertyListRef plist =
        CFPropertyListCreateWithData(kCFAllocatorDefault, cf_data, kCFPropertyListImmutable, NULL, &error);
    CFRelease(cf_data);

    if (error)
        CFRelease(error);

    if (!plist)
        return false;

    if (CFGetTypeID(plist) != CFDictionaryGetTypeID()) {
        CFRelease(plist);
        return false;
    }

    CFDictionaryRef root = (CFDictionaryRef)plist;
    struct macos_powermetrics_sample parsed = {0};

    char pressure[64] = "";
    if (cf_dictionary_get_cstring(root, CFSTR("thermal_pressure"), pressure, sizeof(pressure))) {
        parsed.has_thermal_pressure = true;
        parsed.thermal_pressure = macos_thermal_pressure_from_string(pressure);
    }

    CFTypeRef smc_obj = CFDictionaryGetValue(root, CFSTR("smc"));
    if (smc_obj && CFGetTypeID(smc_obj) == CFDictionaryGetTypeID()) {
        CFDictionaryRef smc = (CFDictionaryRef)smc_obj;
        parsed.has_fan = cf_dictionary_get_double(smc, CFSTR("fan"), &parsed.fan_rpm);
        parsed.has_cpu_die = cf_dictionary_get_double(smc, CFSTR("cpu_die"), &parsed.cpu_die_c);
        parsed.has_gpu_die = cf_dictionary_get_double(smc, CFSTR("gpu_die"), &parsed.gpu_die_c);
        parsed.has_cpu_thermal_level =
            cf_dictionary_get_uint64(smc, CFSTR("cpu_thermal_level"), &parsed.cpu_thermal_level);
        parsed.has_gpu_thermal_level =
            cf_dictionary_get_uint64(smc, CFSTR("gpu_thermal_level"), &parsed.gpu_thermal_level);
        parsed.has_io_thermal_level =
            cf_dictionary_get_uint64(smc, CFSTR("io_thermal_level"), &parsed.io_thermal_level);
        parsed.has_cpu_prochot = cf_dictionary_get_bool(smc, CFSTR("cpu_prochot"), &parsed.cpu_prochot);
        parsed.has_smc_prochot = cf_dictionary_get_bool(smc, CFSTR("smc_prochot"), &parsed.smc_prochot);
    }

    parsed.has_gpu_power = macos_powermetrics_parse_gpu_power(root, &parsed.gpu_power_w);

    parsed.valid =
        parsed.has_thermal_pressure || parsed.has_fan || parsed.has_cpu_die || parsed.has_gpu_die ||
        parsed.has_cpu_thermal_level || parsed.has_gpu_thermal_level || parsed.has_io_thermal_level ||
        parsed.has_cpu_prochot || parsed.has_smc_prochot || parsed.has_gpu_power;
    parsed.collected_ut = now_monotonic_usec();

    CFRelease(plist);

    if (!parsed.valid)
        return false;

    *sample = parsed;
    return true;
}

static bool macos_powermetrics_run_argv(const char **argv, struct macos_powermetrics_sample *sample)
{
    POPEN_INSTANCE *pi = spawn_popen_run_argv(argv);
    if (!pi)
        return false;

    char *output = NULL;
    size_t output_size = 0;
    bool ok = macos_powermetrics_read_stdout(pi, &output, &output_size, pm.command_timeout_ms);

    int rc;
    if (ok)
        rc = spawn_popen_wait(pi);
    else
        rc = spawn_popen_kill(pi, 1000);

    if (ok && rc == 0)
        ok = macos_powermetrics_parse_plist(output, output_size, sample);
    else
        ok = false;

    freez(output);
    return ok;
}

static void macos_powermetrics_store_sample(const struct macos_powermetrics_sample *sample)
{
    netdata_mutex_lock(&pm.mutex);
    pm.sample = *sample;
    pm.sample_sequence++;
    pm.consecutive_failures = 0;
    pm.failed_permanently = false;
    netdata_mutex_unlock(&pm.mutex);
}

static bool macos_powermetrics_run_probe(
    const struct macos_powermetrics_sampler_spec *spec,
    struct macos_powermetrics_sample *sample)
{
    char interval_ms[32];
    snprintfz(interval_ms, sizeof(interval_ms), "%d", pm.sample_window_ms);

    const char *argv_ndsudo[] = {
        pm.command,
        spec->ndsudo_probe_command,
        "--sampleWindowMs",
        interval_ms,
        NULL,
    };

    const char *argv_direct[] = {
        pm.command,
        "-n",
        "1",
        "-i",
        interval_ms,
        "-s",
        spec->samplers,
        "-f",
        "plist",
        NULL,
    };

    return macos_powermetrics_run_argv(pm.use_ndsudo ? argv_ndsudo : argv_direct, sample);
}

static const struct macos_powermetrics_sampler_spec *macos_powermetrics_probe_sampler(void)
{
    for (size_t i = 0; i < _countof(macos_powermetrics_sampler_specs); i++) {
        struct macos_powermetrics_sample sample = {0};
        if (!macos_powermetrics_run_probe(&macos_powermetrics_sampler_specs[i], &sample))
            continue;

        macos_powermetrics_store_sample(&sample);
        return &macos_powermetrics_sampler_specs[i];
    }

    return NULL;
}

static POPEN_INSTANCE *macos_powermetrics_start_loop(const struct macos_powermetrics_sampler_spec *spec)
{
    char interval_ms[32];
    snprintfz(interval_ms, sizeof(interval_ms), "%d", pm.sample_interval_ms);

    const char *argv_ndsudo[] = {
        pm.command,
        spec->ndsudo_loop_command,
        "--sampleIntervalMs",
        interval_ms,
        NULL,
    };

    const char *argv_direct[] = {
        pm.command,
        "-n",
        "0",
        "-b",
        "0",
        "-i",
        interval_ms,
        "-s",
        spec->samplers,
        "-f",
        "plist",
        NULL,
    };

    return spawn_popen_run_argv(pm.use_ndsudo ? argv_ndsudo : argv_direct);
}

static bool macos_powermetrics_process_stream_document(const char *data, size_t size)
{
    struct macos_powermetrics_sample sample = {0};
    if (!macos_powermetrics_parse_plist(data, size, &sample))
        return false;

    macos_powermetrics_store_sample(&sample);
    return true;
}

static bool macos_powermetrics_process_stream_buffer(char *buf, size_t *used, bool *stored_sample)
{
    size_t start = 0;
    *stored_sample = false;

    for (size_t i = 0; i < *used; i++) {
        if (buf[i] != '\0')
            continue;

        size_t document_size = i - start;
        if (document_size > 0) {
            if (!macos_powermetrics_process_stream_document(&buf[start], document_size))
                return false;
            *stored_sample = true;
        }

        start = i + 1;
    }

    if (start > 0) {
        *used -= start;
        memmove(buf, &buf[start], *used);
        buf[*used] = '\0';
    }

    return *used < MACOS_POWERMETRICS_MAX_OUTPUT;
}

static bool macos_powermetrics_run_loop(const struct macos_powermetrics_sampler_spec *spec)
{
    POPEN_INSTANCE *pi = macos_powermetrics_start_loop(spec);
    if (!pi)
        return false;

    int fd = spawn_popen_read_fd(pi);
    if (fd < 0) {
        spawn_popen_kill(pi, pm.command_timeout_ms);
        return false;
    }

    char *buf = NULL;
    size_t used = 0, size = 0;
    usec_t last_sample_ut = now_monotonic_usec();
    bool ok = true;

    while (!nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS)) {
        usec_t now_ut = now_monotonic_usec();
        usec_t stale_after_ut =
            last_sample_ut + (usec_t)(pm.sample_interval_ms + pm.command_timeout_ms) * USEC_PER_MS;
        if (now_ut >= stale_after_ut) {
            ok = false;
            break;
        }

        int remaining_ms = (int)((stale_after_ut - now_ut) / USEC_PER_MS);
        if (remaining_ms > MACOS_POWERMETRICS_READ_STEP_MS)
            remaining_ms = MACOS_POWERMETRICS_READ_STEP_MS;
        if (remaining_ms < 1)
            remaining_ms = 1;

        struct pollfd pfd = {
            .fd = fd,
            .events = POLLIN | POLLHUP,
        };

        int rc = poll(&pfd, 1, remaining_ms);
        if (rc < 0) {
            if (errno == EINTR)
                continue;
            ok = false;
            break;
        }

        if (rc == 0)
            continue;

        if (pfd.revents & POLLIN) {
            char tmp[4096];
            ssize_t bytes = read(fd, tmp, sizeof(tmp));
            if (bytes < 0) {
                if (errno == EINTR)
                    continue;
                ok = false;
                break;
            }

            if (bytes == 0) {
                ok = false;
                break;
            }

            if (!macos_powermetrics_append(&buf, &used, &size, tmp, (size_t)bytes)) {
                ok = false;
                break;
            }

            bool stored_sample = false;
            if (!macos_powermetrics_process_stream_buffer(buf, &used, &stored_sample)) {
                ok = false;
                break;
            }

            if (stored_sample)
                last_sample_ut = now_monotonic_usec();
        }

        if ((pfd.revents & POLLHUP) && !(pfd.revents & POLLIN)) {
            ok = false;
            break;
        }

        if (pfd.revents & (POLLERR | POLLNVAL)) {
            ok = false;
            break;
        }
    }

    freez(buf);
    spawn_popen_kill(pi, pm.command_timeout_ms);
    return ok && !nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS);
}

static void macos_powermetrics_sleep_interruptibly(int timeout_ms)
{
    while (timeout_ms > 0 && !nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS)) {
        int step_ms = timeout_ms > 1000 ? 1000 : timeout_ms;
        sleep_usec((usec_t)step_ms * USEC_PER_MS);
        timeout_ms -= step_ms;
    }
}

static void macos_powermetrics_thread(void *ptr __maybe_unused)
{
    nd_thread_tag_set("macos-pwrmet");

    int backoff_ms = MACOS_POWERMETRICS_INITIAL_BACKOFF_MS;

    while (!nd_thread_signaled_to_cancel() && service_running(SERVICE_COLLECTORS)) {
        const struct macos_powermetrics_sampler_spec *spec = macos_powermetrics_probe_sampler();
        if (!spec) {
            netdata_mutex_lock(&pm.mutex);
            if (pm.sample_sequence == 0 && ++pm.consecutive_failures >= MACOS_POWERMETRICS_STARTUP_FAILURES_MAX)
                pm.failed_permanently = true;

            bool failed_permanently = pm.failed_permanently;
            bool should_log = failed_permanently && !pm.logged_unavailable;
            if (should_log)
                pm.logged_unavailable = true;
            netdata_mutex_unlock(&pm.mutex);

            if (should_log)
                collector_error(
                    "MACOS: disabling powermetrics thermal/fan collection after repeated sampler probe failures; "
                    "this usually means powermetrics is unavailable, netdata cannot run it with sufficient privileges, "
                    "or this macOS version does not expose the requested sampler");

            if (failed_permanently)
                break;

            macos_powermetrics_sleep_interruptibly(backoff_ms);
            if (backoff_ms < MACOS_POWERMETRICS_MAX_BACKOFF_MS)
                backoff_ms *= 2;
            if (backoff_ms > MACOS_POWERMETRICS_MAX_BACKOFF_MS)
                backoff_ms = MACOS_POWERMETRICS_MAX_BACKOFF_MS;
            continue;
        }

        uint64_t sequence_before_loop;
        netdata_mutex_lock(&pm.mutex);
        sequence_before_loop = pm.sample_sequence;
        netdata_mutex_unlock(&pm.mutex);

        (void)macos_powermetrics_run_loop(spec);
        if (nd_thread_signaled_to_cancel() || !service_running(SERVICE_COLLECTORS))
            break;

        bool should_log_loop_failure = false;
        uint64_t sequence_after_loop;
        netdata_mutex_lock(&pm.mutex);
        sequence_after_loop = pm.sample_sequence;
        if (!pm.logged_loop_failure) {
            pm.logged_loop_failure = true;
            should_log_loop_failure = true;
        }
        netdata_mutex_unlock(&pm.mutex);

        if (should_log_loop_failure)
            collector_error("MACOS: powermetrics loop stopped; probing samplers again before restarting loop mode");

        bool loop_produced_sample = sequence_after_loop > sequence_before_loop;
        macos_powermetrics_sleep_interruptibly(
            loop_produced_sample ? MACOS_POWERMETRICS_INITIAL_BACKOFF_MS : backoff_ms);
        if (loop_produced_sample)
            backoff_ms = MACOS_POWERMETRICS_INITIAL_BACKOFF_MS;
        else {
            if (backoff_ms < MACOS_POWERMETRICS_MAX_BACKOFF_MS)
                backoff_ms *= 2;
            if (backoff_ms > MACOS_POWERMETRICS_MAX_BACKOFF_MS)
                backoff_ms = MACOS_POWERMETRICS_MAX_BACKOFF_MS;
        }
    }
}

static void macos_powermetrics_init(void)
{
    if (pm.initialized)
        return;

    netdata_mutex_init(&pm.mutex);

    pm.sample_interval_ms = (int)inicfg_get_duration_ms(
        &netdata_config,
        "plugin:macos:powermetrics",
        "sample every",
        MACOS_POWERMETRICS_DEFAULT_SAMPLE_EVERY * MSEC_PER_SEC);
    if (pm.sample_interval_ms < (int)MSEC_PER_SEC)
        pm.sample_interval_ms = MSEC_PER_SEC;
    if (pm.sample_interval_ms > MACOS_POWERMETRICS_INTERVAL_MS_MAX)
        pm.sample_interval_ms = MACOS_POWERMETRICS_INTERVAL_MS_MAX;

    pm.sample_window_ms = (int)inicfg_get_duration_ms(
        &netdata_config,
        "plugin:macos:powermetrics",
        "sample window",
        MACOS_POWERMETRICS_DEFAULT_SAMPLE_WINDOW_MS);
    if (pm.sample_window_ms < 100)
        pm.sample_window_ms = 100;
    if (pm.sample_window_ms > MACOS_POWERMETRICS_INTERVAL_MS_MAX)
        pm.sample_window_ms = MACOS_POWERMETRICS_INTERVAL_MS_MAX;

    pm.command_timeout_ms = (int)inicfg_get_duration_ms(
        &netdata_config,
        "plugin:macos:powermetrics",
        "command timeout",
        MACOS_POWERMETRICS_DEFAULT_TIMEOUT_MS);
    if (pm.command_timeout_ms < pm.sample_window_ms + 1000)
        pm.command_timeout_ms = pm.sample_window_ms + 1000;

    pm.use_ndsudo = inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "use ndsudo", 1);

    const char *command = inicfg_get(
        &netdata_config,
        "plugin:macos:powermetrics",
        "command path",
        MACOS_POWERMETRICS_DEFAULT_COMMAND);

    if (pm.use_ndsudo) {
        if (netdata_configured_primary_plugins_dir && *netdata_configured_primary_plugins_dir)
            snprintfz(pm.command, sizeof(pm.command), "%s/ndsudo", netdata_configured_primary_plugins_dir);
        else
            snprintfz(pm.command, sizeof(pm.command), "ndsudo");
    } else
        snprintfz(pm.command, sizeof(pm.command), "%s", command && *command ? command : MACOS_POWERMETRICS_DEFAULT_COMMAND);

    pm.thread = nd_thread_create("MACPWRMET", NETDATA_THREAD_OPTION_DEFAULT, macos_powermetrics_thread, NULL);
    if (!pm.thread) {
        pm.failed_permanently = true;
        collector_error("MACOS: cannot start powermetrics collection thread");
    }
    pm.initialized = true;
}

static void macos_powermetrics_update_thermal_pressure(const struct macos_powermetrics_sample *sample, int update_every)
{
    if (!st_thermal_pressure) {
        st_thermal_pressure = rrdset_create_localhost(
            "macos",
            "thermal_pressure",
            NULL,
            "thermal",
            "macos.thermal_pressure",
            "Thermal Pressure",
            "state",
            "macos.plugin",
            "powermetrics",
            NETDATA_CHART_PRIO_SENSORS - 10,
            update_every,
            RRDSET_TYPE_LINE);

        for (size_t i = 0; i < MACOS_THERMAL_PRESSURE_COUNT; i++)
            rd_thermal_pressure[i] =
                rrddim_add(st_thermal_pressure, macos_thermal_pressure_names[i], NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    for (size_t i = 0; i < MACOS_THERMAL_PRESSURE_COUNT; i++)
        rrddim_set_by_pointer(
            st_thermal_pressure,
            rd_thermal_pressure[i],
            sample->thermal_pressure == (enum macos_thermal_pressure)i ? 1 : 0);

    rrdset_done(st_thermal_pressure);
}

static void macos_powermetrics_update_sensor(
    RRDSET **st,
    RRDDIM **rd,
    const char *id,
    const char *title,
    const char *family,
    const char *context,
    const char *units,
    const char *sensor_label,
    int priority,
    int update_every,
    collected_number value,
    collected_number divisor)
{
    if (!*st) {
        *st = rrdset_create_localhost(
            "sensors",
            id,
            NULL,
            family,
            context,
            title,
            units,
            "macos.plugin",
            "powermetrics",
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add((*st)->rrdlabels, "source", "powermetrics", RRDLABEL_SRC_AUTO);
        rrdlabels_add((*st)->rrdlabels, "sensor", sensor_label, RRDLABEL_SRC_AUTO);
    }

    if (!*rd)
        *rd = rrddim_add(*st, "input", NULL, 1, divisor, RRD_ALGORITHM_ABSOLUTE);

    rrddim_set_by_pointer(*st, *rd, value);
    rrdset_done(*st);
}

static void macos_powermetrics_update_thermal_levels(const struct macos_powermetrics_sample *sample, int update_every)
{
    if (!st_smc_thermal_level) {
        st_smc_thermal_level = rrdset_create_localhost(
            "macos",
            "smc_thermal_level",
            NULL,
            "thermal",
            "macos.smc_thermal_level",
            "SMC Thermal Levels",
            "level",
            "macos.plugin",
            "powermetrics",
            NETDATA_CHART_PRIO_SENSORS - 9,
            update_every,
            RRDSET_TYPE_LINE);

        rd_smc_cpu_thermal_level = rrddim_add(st_smc_thermal_level, "cpu", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_smc_gpu_thermal_level = rrddim_add(st_smc_thermal_level, "gpu", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_smc_io_thermal_level = rrddim_add(st_smc_thermal_level, "io", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    // Set every dimension each sample (0 when the sampler omitted it) so the
    // chart reflects the current sample instead of latching the previous value.
    rrddim_set_by_pointer(
        st_smc_thermal_level,
        rd_smc_cpu_thermal_level,
        sample->has_cpu_thermal_level ? (collected_number)sample->cpu_thermal_level : 0);
    rrddim_set_by_pointer(
        st_smc_thermal_level,
        rd_smc_gpu_thermal_level,
        sample->has_gpu_thermal_level ? (collected_number)sample->gpu_thermal_level : 0);
    rrddim_set_by_pointer(
        st_smc_thermal_level,
        rd_smc_io_thermal_level,
        sample->has_io_thermal_level ? (collected_number)sample->io_thermal_level : 0);

    rrdset_done(st_smc_thermal_level);
}

static void macos_powermetrics_update_prochot(const struct macos_powermetrics_sample *sample, int update_every)
{
    if (!st_smc_prochot) {
        st_smc_prochot = rrdset_create_localhost(
            "macos",
            "smc_prochot",
            NULL,
            "thermal",
            "macos.smc_prochot",
            "SMC Processor Hot Assertions",
            "status",
            "macos.plugin",
            "powermetrics",
            NETDATA_CHART_PRIO_SENSORS - 8,
            update_every,
            RRDSET_TYPE_LINE);

        rd_smc_cpu_prochot = rrddim_add(st_smc_prochot, "cpu", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_smc_prochot = rrddim_add(st_smc_prochot, "smc", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_smc_prochot, rd_smc_cpu_prochot, sample->has_cpu_prochot ? (sample->cpu_prochot ? 1 : 0) : 0);
    rrddim_set_by_pointer(st_smc_prochot, rd_smc_prochot, sample->has_smc_prochot ? (sample->smc_prochot ? 1 : 0) : 0);

    rrdset_done(st_smc_prochot);
}

static void macos_powermetrics_update_gpu_power(const struct macos_powermetrics_sample *sample, int update_every)
{
    if (!st_gpu_power) {
        st_gpu_power = rrdset_create_localhost(
            "macos",
            "gpu_power",
            NULL,
            "gpu",
            "macos.gpu_power",
            "GPU Power",
            "W",
            "macos.plugin",
            "powermetrics",
            NETDATA_CHART_PRIO_SENSORS - 18,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(st_gpu_power->rrdlabels, "source", "powermetrics", RRDLABEL_SRC_AUTO);
        rd_gpu_power = rrddim_add(st_gpu_power, "power", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_gpu_power, rd_gpu_power, (collected_number)llround(sample->gpu_power_w * 1000.0));
    rrdset_done(st_gpu_power);
}

int do_macos_powermetrics(int update_every, usec_t dt __maybe_unused)
{
    static int do_thermal_pressure = -1, do_smc_fan = -1, do_smc_temperatures = -1,
               do_smc_thermal_levels = -1, do_smc_prochot = -1, do_gpu_power = -1;

    if (unlikely(do_thermal_pressure == -1)) {
        do_thermal_pressure =
            inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "thermal pressure", 1);
        do_smc_fan = inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "SMC fan speed", 1);
        do_smc_temperatures =
            inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "SMC temperatures", 1);
        do_smc_thermal_levels =
            inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "SMC thermal levels", 1);
        do_smc_prochot = inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "SMC prochot", 1);
        do_gpu_power = inicfg_get_boolean(&netdata_config, "plugin:macos:powermetrics", "GPU power", 1);

        if (!do_thermal_pressure && !do_smc_fan && !do_smc_temperatures && !do_smc_thermal_levels &&
            !do_smc_prochot && !do_gpu_power)
            return 1;

        macos_powermetrics_init();
    }

    struct macos_powermetrics_sample sample = {0};
    bool failed_permanently;
    uint64_t sample_sequence;
    static uint64_t last_sample_sequence = 0;

    netdata_mutex_lock(&pm.mutex);
    sample = pm.sample;
    failed_permanently = pm.failed_permanently;
    sample_sequence = pm.sample_sequence;
    netdata_mutex_unlock(&pm.mutex);

    if (failed_permanently) {
        macos_powermetrics_mark_charts_obsolete();
        // Join the background thread and release the mutex now, instead of
        // deferring to full plugin shutdown, so disabling the module does not
        // leak the worker thread until exit.
        macos_powermetrics_cleanup();
        return 1;
    }

    if (!sample.valid || sample_sequence == 0 || sample_sequence == last_sample_sequence)
        return 0;
    last_sample_sequence = sample_sequence;

    if (do_thermal_pressure && sample.has_thermal_pressure)
        macos_powermetrics_update_thermal_pressure(&sample, update_every);

    if (do_smc_fan && sample.has_fan)
        macos_powermetrics_update_sensor(
            &st_fan,
            &rd_fan,
            "macos_fan_speed",
            "Sensor Fan Speed",
            "Fan",
            "system.hw.sensor.fan.input",
            "rotations per minute",
            "fan",
            NETDATA_CHART_PRIO_SENSORS + 5,
            update_every,
            (collected_number)llround(sample.fan_rpm),
            1);

    if (do_smc_temperatures && sample.has_cpu_die)
        macos_powermetrics_update_sensor(
            &st_cpu_die,
            &rd_cpu_die,
            "macos_cpu_die_temperature",
            "CPU Die Temperature",
            "Temperature",
            "system.hw.sensor.temperature.input",
            "degrees Celsius",
            "cpu_die",
            NETDATA_CHART_PRIO_SENSORS,
            update_every,
            (collected_number)llround(sample.cpu_die_c * 1000.0),
            1000);

    if (do_smc_temperatures && sample.has_gpu_die && !macos_gpu_temperature_available())
        macos_powermetrics_update_sensor(
            &st_gpu_die,
            &rd_gpu_die,
            "macos_gpu_die_temperature",
            "GPU Die Temperature",
            "Temperature",
            "system.hw.sensor.temperature.input",
            "degrees Celsius",
            "gpu_die",
            NETDATA_CHART_PRIO_SENSORS + 1,
            update_every,
            (collected_number)llround(sample.gpu_die_c * 1000.0),
            1000);

    if (do_smc_thermal_levels &&
        (sample.has_cpu_thermal_level || sample.has_gpu_thermal_level || sample.has_io_thermal_level))
        macos_powermetrics_update_thermal_levels(&sample, update_every);

    if (do_smc_prochot && (sample.has_cpu_prochot || sample.has_smc_prochot))
        macos_powermetrics_update_prochot(&sample, update_every);

    if (do_gpu_power && sample.has_gpu_power && !macos_gpu_ioreport_available())
        macos_powermetrics_update_gpu_power(&sample, update_every);

    return 0;
}

void macos_powermetrics_cleanup(void)
{
    if (!pm.initialized)
        return;

    if (pm.thread) {
        nd_thread_signal_cancel(pm.thread);
        nd_thread_join(pm.thread);
        pm.thread = NULL;
    }

    macos_powermetrics_mark_charts_obsolete();
    netdata_mutex_destroy(&pm.mutex);
    pm.initialized = false;
}
