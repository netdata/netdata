// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#ifdef ENABLE_SYSTEMD_DBUS
#include <systemd/sd-bus.h>

#define SYSTEMD_UNITS_MAX_PARAMS 10
#define SYSTEMD_UNITS_DBUS_TYPES "(ssssssouso)"

// ----------------------------------------------------------------------------
// copied from systemd: string-table.h

typedef char sd_char;
#define XCONCATENATE(x, y) x ## y
#define CONCATENATE(x, y) XCONCATENATE(x, y)

#ifndef __COVERITY__
#  define VOID_0 ((void)0)
#else
#  define VOID_0 ((void*)0)
#endif

#define ELEMENTSOF(x)                                                   \
        (__builtin_choose_expr(                                         \
                !__builtin_types_compatible_p(typeof(x), typeof(&*(x))), \
                sizeof(x)/sizeof((x)[0]),                               \
                VOID_0))

#define UNIQ_T(x, uniq) CONCATENATE(__unique_prefix_, CONCATENATE(x, uniq))
#define UNIQ __COUNTER__
#define __CMP(aq, a, bq, b)                             \
        ({                                              \
                const typeof(a) UNIQ_T(A, aq) = (a);    \
                const typeof(b) UNIQ_T(B, bq) = (b);    \
                UNIQ_T(A, aq) < UNIQ_T(B, bq) ? -1 :    \
                UNIQ_T(A, aq) > UNIQ_T(B, bq) ? 1 : 0;  \
        })
#define CMP(a, b) __CMP(UNIQ, (a), UNIQ, (b))

static inline int strcmp_ptr(const sd_char *a, const sd_char *b) {
    if (a && b)
        return strcmp(a, b);

    return CMP(a, b);
}

static inline bool streq_ptr(const sd_char *a, const sd_char *b) {
    return strcmp_ptr(a, b) == 0;
}

ssize_t string_table_lookup(const char * const *table, size_t len, const char *key) {
    if (!key)
        return -EINVAL;

    for (size_t i = 0; i < len; ++i)
        if (streq_ptr(table[i], key))
            return (ssize_t) i;

    return -EINVAL;
}

/* For basic lookup tables with strictly enumerated entries */
#define _DEFINE_STRING_TABLE_LOOKUP_TO_STRING(name,type,scope)          \
        scope const char *name##_to_string(type i) {                    \
                if (i < 0 || i >= (type) ELEMENTSOF(name##_table))      \
                        return NULL;                                    \
                return name##_table[i];                                 \
        }

#define _DEFINE_STRING_TABLE_LOOKUP_FROM_STRING(name,type,scope)        \
        scope type name##_from_string(const char *s) {                  \
                return (type) string_table_lookup(name##_table, ELEMENTSOF(name##_table), s); \
        }

#define _DEFINE_STRING_TABLE_LOOKUP(name,type,scope)                    \
        _DEFINE_STRING_TABLE_LOOKUP_TO_STRING(name,type,scope)          \
        _DEFINE_STRING_TABLE_LOOKUP_FROM_STRING(name,type,scope)

#define DEFINE_STRING_TABLE_LOOKUP(name,type) _DEFINE_STRING_TABLE_LOOKUP(name,type,)

// ----------------------------------------------------------------------------
// copied from systemd: unit-def.h

typedef enum UnitType {
    UNIT_SERVICE,
    UNIT_MOUNT,
    UNIT_SWAP,
    UNIT_SOCKET,
    UNIT_TARGET,
    UNIT_DEVICE,
    UNIT_AUTOMOUNT,
    UNIT_TIMER,
    UNIT_PATH,
    UNIT_SLICE,
    UNIT_SCOPE,
    _UNIT_TYPE_MAX,
    _UNIT_TYPE_INVALID = -EINVAL,
} UnitType;

typedef enum UnitLoadState {
    UNIT_STUB,
    UNIT_LOADED,
    UNIT_NOT_FOUND,    /* error condition #1: unit file not found */
    UNIT_BAD_SETTING,  /* error condition #2: we couldn't parse some essential unit file setting */
    UNIT_ERROR,        /* error condition #3: other "system" error, catchall for the rest */
    UNIT_MERGED,
    UNIT_MASKED,
    _UNIT_LOAD_STATE_MAX,
    _UNIT_LOAD_STATE_INVALID = -EINVAL,
} UnitLoadState;

typedef enum UnitActiveState {
    UNIT_ACTIVE,
    UNIT_RELOADING,
    UNIT_INACTIVE,
    UNIT_FAILED,
    UNIT_ACTIVATING,
    UNIT_DEACTIVATING,
    UNIT_MAINTENANCE,
    _UNIT_ACTIVE_STATE_MAX,
    _UNIT_ACTIVE_STATE_INVALID = -EINVAL,
} UnitActiveState;

typedef enum AutomountState {
    AUTOMOUNT_DEAD,
    AUTOMOUNT_WAITING,
    AUTOMOUNT_RUNNING,
    AUTOMOUNT_FAILED,
    _AUTOMOUNT_STATE_MAX,
    _AUTOMOUNT_STATE_INVALID = -EINVAL,
} AutomountState;

typedef enum DeviceState {
    DEVICE_DEAD,
    DEVICE_TENTATIVE, /* mounted or swapped, but not (yet) announced by udev */
    DEVICE_PLUGGED,   /* announced by udev */
    _DEVICE_STATE_MAX,
    _DEVICE_STATE_INVALID = -EINVAL,
} DeviceState;

typedef enum MountState {
    MOUNT_DEAD,
    MOUNT_MOUNTING,               /* /usr/bin/mount is running, but the mount is not done yet. */
    MOUNT_MOUNTING_DONE,          /* /usr/bin/mount is running, and the mount is done. */
    MOUNT_MOUNTED,
    MOUNT_REMOUNTING,
    MOUNT_UNMOUNTING,
    MOUNT_REMOUNTING_SIGTERM,
    MOUNT_REMOUNTING_SIGKILL,
    MOUNT_UNMOUNTING_SIGTERM,
    MOUNT_UNMOUNTING_SIGKILL,
    MOUNT_FAILED,
    MOUNT_CLEANING,
    _MOUNT_STATE_MAX,
    _MOUNT_STATE_INVALID = -EINVAL,
} MountState;

typedef enum PathState {
    PATH_DEAD,
    PATH_WAITING,
    PATH_RUNNING,
    PATH_FAILED,
    _PATH_STATE_MAX,
    _PATH_STATE_INVALID = -EINVAL,
} PathState;

typedef enum ScopeState {
    SCOPE_DEAD,
    SCOPE_START_CHOWN,
    SCOPE_RUNNING,
    SCOPE_ABANDONED,
    SCOPE_STOP_SIGTERM,
    SCOPE_STOP_SIGKILL,
    SCOPE_FAILED,
    _SCOPE_STATE_MAX,
    _SCOPE_STATE_INVALID = -EINVAL,
} ScopeState;

typedef enum ServiceState {
    SERVICE_DEAD,
    SERVICE_CONDITION,
    SERVICE_START_PRE,
    SERVICE_START,
    SERVICE_START_POST,
    SERVICE_RUNNING,
    SERVICE_EXITED,            /* Nothing is running anymore, but RemainAfterExit is true hence this is OK */
    SERVICE_RELOAD,            /* Reloading via ExecReload= */
    SERVICE_RELOAD_SIGNAL,     /* Reloading via SIGHUP requested */
    SERVICE_RELOAD_NOTIFY,     /* Waiting for READY=1 after RELOADING=1 notify */
    SERVICE_STOP,              /* No STOP_PRE state, instead just register multiple STOP executables */
    SERVICE_STOP_WATCHDOG,
    SERVICE_STOP_SIGTERM,
    SERVICE_STOP_SIGKILL,
    SERVICE_STOP_POST,
    SERVICE_FINAL_WATCHDOG,    /* In case the STOP_POST executable needs to be aborted. */
    SERVICE_FINAL_SIGTERM,     /* In case the STOP_POST executable hangs, we shoot that down, too */
    SERVICE_FINAL_SIGKILL,
    SERVICE_FAILED,
    SERVICE_DEAD_BEFORE_AUTO_RESTART,
    SERVICE_FAILED_BEFORE_AUTO_RESTART,
    SERVICE_DEAD_RESOURCES_PINNED,  /* Like SERVICE_DEAD, but with pinned resources */
    SERVICE_AUTO_RESTART,
    SERVICE_AUTO_RESTART_QUEUED,
    SERVICE_CLEANING,
    _SERVICE_STATE_MAX,
    _SERVICE_STATE_INVALID = -EINVAL,
} ServiceState;

typedef enum SliceState {
    SLICE_DEAD,
    SLICE_ACTIVE,
    _SLICE_STATE_MAX,
    _SLICE_STATE_INVALID = -EINVAL,
} SliceState;

typedef enum SocketState {
    SOCKET_DEAD,
    SOCKET_START_PRE,
    SOCKET_START_CHOWN,
    SOCKET_START_POST,
    SOCKET_LISTENING,
    SOCKET_RUNNING,
    SOCKET_STOP_PRE,
    SOCKET_STOP_PRE_SIGTERM,
    SOCKET_STOP_PRE_SIGKILL,
    SOCKET_STOP_POST,
    SOCKET_FINAL_SIGTERM,
    SOCKET_FINAL_SIGKILL,
    SOCKET_FAILED,
    SOCKET_CLEANING,
    _SOCKET_STATE_MAX,
    _SOCKET_STATE_INVALID = -EINVAL,
} SocketState;

typedef enum SwapState {
    SWAP_DEAD,
    SWAP_ACTIVATING,               /* /sbin/swapon is running, but the swap not yet enabled. */
    SWAP_ACTIVATING_DONE,          /* /sbin/swapon is running, and the swap is done. */
    SWAP_ACTIVE,
    SWAP_DEACTIVATING,
    SWAP_DEACTIVATING_SIGTERM,
    SWAP_DEACTIVATING_SIGKILL,
    SWAP_FAILED,
    SWAP_CLEANING,
    _SWAP_STATE_MAX,
    _SWAP_STATE_INVALID = -EINVAL,
} SwapState;

typedef enum TargetState {
    TARGET_DEAD,
    TARGET_ACTIVE,
    _TARGET_STATE_MAX,
    _TARGET_STATE_INVALID = -EINVAL,
} TargetState;

typedef enum TimerState {
    TIMER_DEAD,
    TIMER_WAITING,
    TIMER_RUNNING,
    TIMER_ELAPSED,
    TIMER_FAILED,
    _TIMER_STATE_MAX,
    _TIMER_STATE_INVALID = -EINVAL,
} TimerState;

// ----------------------------------------------------------------------------
// copied from systemd: unit-def.c

static const char* const unit_type_table[_UNIT_TYPE_MAX] = {
        [UNIT_SERVICE]   = "service",
        [UNIT_SOCKET]    = "socket",
        [UNIT_TARGET]    = "target",
        [UNIT_DEVICE]    = "device",
        [UNIT_MOUNT]     = "mount",
        [UNIT_AUTOMOUNT] = "automount",
        [UNIT_SWAP]      = "swap",
        [UNIT_TIMER]     = "timer",
        [UNIT_PATH]      = "path",
        [UNIT_SLICE]     = "slice",
        [UNIT_SCOPE]     = "scope",
};

DEFINE_STRING_TABLE_LOOKUP(unit_type, UnitType);

static const char* const unit_load_state_table[_UNIT_LOAD_STATE_MAX] = {
        [UNIT_STUB]        = "stub",
        [UNIT_LOADED]      = "loaded",
        [UNIT_NOT_FOUND]   = "not-found",
        [UNIT_BAD_SETTING] = "bad-setting",
        [UNIT_ERROR]       = "error",
        [UNIT_MERGED]      = "merged",
        [UNIT_MASKED]      = "masked"
};

DEFINE_STRING_TABLE_LOOKUP(unit_load_state, UnitLoadState);

static const char* const unit_active_state_table[_UNIT_ACTIVE_STATE_MAX] = {
        [UNIT_ACTIVE]       = "active",
        [UNIT_RELOADING]    = "reloading",
        [UNIT_INACTIVE]     = "inactive",
        [UNIT_FAILED]       = "failed",
        [UNIT_ACTIVATING]   = "activating",
        [UNIT_DEACTIVATING] = "deactivating",
        [UNIT_MAINTENANCE]  = "maintenance",
};

DEFINE_STRING_TABLE_LOOKUP(unit_active_state, UnitActiveState);

static const char* const automount_state_table[_AUTOMOUNT_STATE_MAX] = {
        [AUTOMOUNT_DEAD]    = "dead",
        [AUTOMOUNT_WAITING] = "waiting",
        [AUTOMOUNT_RUNNING] = "running",
        [AUTOMOUNT_FAILED]  = "failed"
};

DEFINE_STRING_TABLE_LOOKUP(automount_state, AutomountState);

static const char* const device_state_table[_DEVICE_STATE_MAX] = {
        [DEVICE_DEAD]      = "dead",
        [DEVICE_TENTATIVE] = "tentative",
        [DEVICE_PLUGGED]   = "plugged",
};

DEFINE_STRING_TABLE_LOOKUP(device_state, DeviceState);

static const char* const mount_state_table[_MOUNT_STATE_MAX] = {
        [MOUNT_DEAD]               = "dead",
        [MOUNT_MOUNTING]           = "mounting",
        [MOUNT_MOUNTING_DONE]      = "mounting-done",
        [MOUNT_MOUNTED]            = "mounted",
        [MOUNT_REMOUNTING]         = "remounting",
        [MOUNT_UNMOUNTING]         = "unmounting",
        [MOUNT_REMOUNTING_SIGTERM] = "remounting-sigterm",
        [MOUNT_REMOUNTING_SIGKILL] = "remounting-sigkill",
        [MOUNT_UNMOUNTING_SIGTERM] = "unmounting-sigterm",
        [MOUNT_UNMOUNTING_SIGKILL] = "unmounting-sigkill",
        [MOUNT_FAILED]             = "failed",
        [MOUNT_CLEANING]           = "cleaning",
};

DEFINE_STRING_TABLE_LOOKUP(mount_state, MountState);

static const char* const path_state_table[_PATH_STATE_MAX] = {
        [PATH_DEAD]    = "dead",
        [PATH_WAITING] = "waiting",
        [PATH_RUNNING] = "running",
        [PATH_FAILED]  = "failed"
};

DEFINE_STRING_TABLE_LOOKUP(path_state, PathState);

static const char* const scope_state_table[_SCOPE_STATE_MAX] = {
        [SCOPE_DEAD]         = "dead",
        [SCOPE_START_CHOWN]  = "start-chown",
        [SCOPE_RUNNING]      = "running",
        [SCOPE_ABANDONED]    = "abandoned",
        [SCOPE_STOP_SIGTERM] = "stop-sigterm",
        [SCOPE_STOP_SIGKILL] = "stop-sigkill",
        [SCOPE_FAILED]       = "failed",
};

DEFINE_STRING_TABLE_LOOKUP(scope_state, ScopeState);

static const char* const service_state_table[_SERVICE_STATE_MAX] = {
        [SERVICE_DEAD]                       = "dead",
        [SERVICE_CONDITION]                  = "condition",
        [SERVICE_START_PRE]                  = "start-pre",
        [SERVICE_START]                      = "start",
        [SERVICE_START_POST]                 = "start-post",
        [SERVICE_RUNNING]                    = "running",
        [SERVICE_EXITED]                     = "exited",
        [SERVICE_RELOAD]                     = "reload",
        [SERVICE_RELOAD_SIGNAL]              = "reload-signal",
        [SERVICE_RELOAD_NOTIFY]              = "reload-notify",
        [SERVICE_STOP]                       = "stop",
        [SERVICE_STOP_WATCHDOG]              = "stop-watchdog",
        [SERVICE_STOP_SIGTERM]               = "stop-sigterm",
        [SERVICE_STOP_SIGKILL]               = "stop-sigkill",
        [SERVICE_STOP_POST]                  = "stop-post",
        [SERVICE_FINAL_WATCHDOG]             = "final-watchdog",
        [SERVICE_FINAL_SIGTERM]              = "final-sigterm",
        [SERVICE_FINAL_SIGKILL]              = "final-sigkill",
        [SERVICE_FAILED]                     = "failed",
        [SERVICE_DEAD_BEFORE_AUTO_RESTART]   = "dead-before-auto-restart",
        [SERVICE_FAILED_BEFORE_AUTO_RESTART] = "failed-before-auto-restart",
        [SERVICE_DEAD_RESOURCES_PINNED]      = "dead-resources-pinned",
        [SERVICE_AUTO_RESTART]               = "auto-restart",
        [SERVICE_AUTO_RESTART_QUEUED]        = "auto-restart-queued",
        [SERVICE_CLEANING]                   = "cleaning",
};

DEFINE_STRING_TABLE_LOOKUP(service_state, ServiceState);

static const char* const slice_state_table[_SLICE_STATE_MAX] = {
        [SLICE_DEAD]   = "dead",
        [SLICE_ACTIVE] = "active"
};

DEFINE_STRING_TABLE_LOOKUP(slice_state, SliceState);

static const char* const socket_state_table[_SOCKET_STATE_MAX] = {
        [SOCKET_DEAD]             = "dead",
        [SOCKET_START_PRE]        = "start-pre",
        [SOCKET_START_CHOWN]      = "start-chown",
        [SOCKET_START_POST]       = "start-post",
        [SOCKET_LISTENING]        = "listening",
        [SOCKET_RUNNING]          = "running",
        [SOCKET_STOP_PRE]         = "stop-pre",
        [SOCKET_STOP_PRE_SIGTERM] = "stop-pre-sigterm",
        [SOCKET_STOP_PRE_SIGKILL] = "stop-pre-sigkill",
        [SOCKET_STOP_POST]        = "stop-post",
        [SOCKET_FINAL_SIGTERM]    = "final-sigterm",
        [SOCKET_FINAL_SIGKILL]    = "final-sigkill",
        [SOCKET_FAILED]           = "failed",
        [SOCKET_CLEANING]         = "cleaning",
};

DEFINE_STRING_TABLE_LOOKUP(socket_state, SocketState);

static const char* const swap_state_table[_SWAP_STATE_MAX] = {
        [SWAP_DEAD]                 = "dead",
        [SWAP_ACTIVATING]           = "activating",
        [SWAP_ACTIVATING_DONE]      = "activating-done",
        [SWAP_ACTIVE]               = "active",
        [SWAP_DEACTIVATING]         = "deactivating",
        [SWAP_DEACTIVATING_SIGTERM] = "deactivating-sigterm",
        [SWAP_DEACTIVATING_SIGKILL] = "deactivating-sigkill",
        [SWAP_FAILED]               = "failed",
        [SWAP_CLEANING]             = "cleaning",
};

DEFINE_STRING_TABLE_LOOKUP(swap_state, SwapState);

static const char* const target_state_table[_TARGET_STATE_MAX] = {
        [TARGET_DEAD]   = "dead",
        [TARGET_ACTIVE] = "active"
};

DEFINE_STRING_TABLE_LOOKUP(target_state, TargetState);

static const char* const timer_state_table[_TIMER_STATE_MAX] = {
        [TIMER_DEAD]    = "dead",
        [TIMER_WAITING] = "waiting",
        [TIMER_RUNNING] = "running",
        [TIMER_ELAPSED] = "elapsed",
        [TIMER_FAILED]  = "failed"
};

DEFINE_STRING_TABLE_LOOKUP(timer_state, TimerState);

typedef struct UnitInfo {
    char *id;
    char *type;
    char *description;
    char *load_state;
    char *active_state;
    char *sub_state;
    char *following;
    char *unit_path;
    uint32_t job_id;
    char *job_type;
    char *job_path;

    UnitType UnitType;
    UnitLoadState UnitLoadState;
    UnitActiveState UnitActiveState;

    union {
        AutomountState AutomountState;
        DeviceState DeviceState;
        MountState MountState;
        PathState PathState;
        ScopeState ScopeState;
        ServiceState ServiceState;
        SliceState SliceState;
        SocketState SocketState;
        SwapState SwapState;
        TargetState TargetState;
        TimerState TimerState;
    };

    FACET_ROW_SEVERITY severity;
    uint32_t prio;

    struct UnitInfo *prev, *next;
} UnitInfo;

int bus_parse_unit_info(sd_bus_message *message, UnitInfo *u) {
    assert(message);
    assert(u);

    u->type = NULL;

    int r = sd_bus_message_read(
            message,
            SYSTEMD_UNITS_DBUS_TYPES,
            &u->id,
            &u->description,
            &u->load_state,
            &u->active_state,
            &u->sub_state,
            &u->following,
            &u->unit_path,
            &u->job_id,
            &u->job_type,
            &u->job_path);

    if(r <= 0)
        return r;

    char *dot;
    if(u->id && (dot = strrchr(u->id, '.')) != NULL)
        u->type = &dot[1];
    else
        u->type = "unknown";

    u->UnitType = unit_type_from_string(u->type);
    u->UnitLoadState = unit_load_state_from_string(u->load_state);
    u->UnitActiveState = unit_active_state_from_string(u->active_state);

    switch(u->UnitType) {
        case UNIT_SERVICE:
            u->ServiceState = service_state_from_string(u->sub_state);
            break;

        case UNIT_MOUNT:
            u->MountState = mount_state_from_string(u->sub_state);
            break;

        case UNIT_SWAP:
            u->SwapState = swap_state_from_string(u->sub_state);
            break;

        case UNIT_SOCKET:
            u->SocketState = socket_state_from_string(u->sub_state);
            break;

        case UNIT_TARGET:
            u->TargetState = target_state_from_string(u->sub_state);
            break;

        case UNIT_DEVICE:
            u->DeviceState = device_state_from_string(u->sub_state);
            break;

        case UNIT_AUTOMOUNT:
            u->AutomountState = automount_state_from_string(u->sub_state);
            break;

        case UNIT_TIMER:
            u->TimerState = timer_state_from_string(u->sub_state);
            break;

        case UNIT_PATH:
            u->PathState = path_state_from_string(u->sub_state);
            break;

        case UNIT_SLICE:
            u->SliceState = slice_state_from_string(u->sub_state);
            break;

        case UNIT_SCOPE:
            u->ScopeState = scope_state_from_string(u->sub_state);
            break;

        default:
            break;
    }

    return r;
}

static void log_dbus_error(int r, const char *msg) {
    netdata_log_error("SYSTEMD_UNITS: %s failed with error %d", msg, r);
}

static int hex_to_int(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return 0;
}

// un-escape hex sequences (\xNN) in id
static void txt_decode(char *txt) {
    if(!txt || !*txt)
        return;

    char *src = txt, *dst = txt;

    size_t id_len = strlen(src);
    size_t s = 0, d = 0;
    for(; s < id_len ; s++) {
        if(src[s] == '\\' && src[s + 1] == 'x' && isxdigit(src[s + 2]) && isxdigit(src[s + 3])) {
            int value = (hex_to_int(src[s + 2]) << 4) + hex_to_int(src[s + 3]);
            dst[d++] = (char)value;
            s += 3;
        }
        else
            dst[d++] = src[s];
    }
    dst[d] = '\0';
}

static UnitInfo *systemd_units_get_all(void) {
    sd_bus *bus = NULL;
    sd_bus_error error = SD_BUS_ERROR_NULL;
    sd_bus_message *reply = NULL;
    UnitInfo *base = NULL;
    int r;

    r = sd_bus_default_system(&bus);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_default_system()");
        return base;
    }

    // This calls the ListUnits method of the org.freedesktop.systemd1.Manager interface
    // Replace "ListUnits" with "ListUnitsFiltered" to get specific units based on filters
    r = sd_bus_call_method(bus,
            "org.freedesktop.systemd1",           /* service to contact */
            "/org/freedesktop/systemd1",          /* object path */
            "org.freedesktop.systemd1.Manager",   /* interface name */
            "ListUnits",                          /* method name */
            &error,                               /* object to return error in */
            &reply,                               /* return message on success */
            NULL);                                /* input signature */
    if (r < 0) {
        log_dbus_error(r, "sd_bus_call_method()");
        return base;
    }

    r = sd_bus_message_enter_container(reply, SD_BUS_TYPE_ARRAY, SYSTEMD_UNITS_DBUS_TYPES);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_enter_container()");
        return base;
    }

    UnitInfo u;
    memset(&u, 0, sizeof(u));
    while ((r = bus_parse_unit_info(reply, &u)) > 0) {
        UnitInfo *i = callocz(1, sizeof(u));
        *i = u;

        i->id = strdupz(u.id && *u.id ? u.id : "-");
        txt_decode(i->id);

        i->type = strdupz(u.type && *u.type ? u.type : "-");
        i->description = strdupz(u.description && *u.description ? u.description : "-");
        txt_decode(i->description);

        i->load_state = strdupz(u.load_state && *u.load_state ? u.load_state : "-");
        i->active_state = strdupz(u.active_state && *u.active_state ? u.active_state : "-");
        i->sub_state = strdupz(u.sub_state && *u.sub_state ? u.sub_state : "-");
        i->following = strdupz(u.following && *u.following ? u.following : "-");
        i->unit_path = strdupz(u.unit_path && *u.unit_path ? u.unit_path : "-");
        i->job_type = strdupz(u.job_type && *u.job_type ? u.job_type : "-");
        i->job_path = strdupz(u.job_path && *u.job_path ? u.job_path : "-");
        i->job_id = u.job_id;

        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(base, i, prev, next);
        memset(&u, 0, sizeof(u));
    }
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_read()");
        return base;
    }

    r = sd_bus_message_exit_container(reply);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_exit_container()");
        return base;
    }

    return base;
}

void systemd_units_free_all(UnitInfo *base) {
    while(base) {
        UnitInfo *u = base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(base, u, prev, next);
        freez((void *)u->id);
        freez((void *)u->type);
        freez((void *)u->description);
        freez((void *)u->load_state);
        freez((void *)u->active_state);
        freez((void *)u->sub_state);
        freez((void *)u->following);
        freez((void *)u->unit_path);
        freez((void *)u->job_type);
        freez((void *)u->job_path);
        freez(u);
    }
}

static void netdata_systemd_units_function_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following parameters are supported:\n"
            "\n"
            "   help\n"
            "      Shows this help message.\n"
            "\n"
            "   info\n"
            "      Request initial configuration information about the plugin.\n"
            "      The key entity returned is the required_params array, which includes\n"
            "      all the available systemd journal sources.\n"
            "      When `info` is requested, all other parameters are ignored.\n"
            "\n"
            , program_name
            , SYSTEMD_UNITS_FUNCTION_NAME
            , SYSTEMD_UNITS_FUNCTION_DESCRIPTION
    );

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

static void netdata_systemd_units_function_info(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_string(wb, "help", SYSTEMD_UNITS_FUNCTION_DESCRIPTION);

    buffer_json_finalize(wb);
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

static void systemd_unit_priority(UnitInfo *u, size_t units) {
    uint32_t prio;

    switch(u->severity) {
        case FACET_ROW_SEVERITY_CRITICAL:
            prio = 0;
            break;

        default:
        case FACET_ROW_SEVERITY_WARNING:
            prio = 1;
            break;

        case FACET_ROW_SEVERITY_NOTICE:
            prio = 2;
            break;

        case FACET_ROW_SEVERITY_NORMAL:
            prio = 3;
            break;

        case FACET_ROW_SEVERITY_DEBUG:
            prio = 4;
            break;
    }

    prio = prio * (uint32_t)(_UNIT_TYPE_MAX + 1) + (uint32_t)u->UnitType;
    u->prio = (prio * units) + u->prio;
}

#define if_less(current, max, target) ({                \
    typeof(current) _wanted = (current);                \
    if((current) < (target))                            \
        _wanted = (target) > (max) ? (max) : (target);  \
    _wanted;                                            \
})

#define if_normal(current, max, target) ({              \
    typeof(current) _wanted = (current);                \
    if((current) == FACET_ROW_SEVERITY_NORMAL)          \
        _wanted = (target) > (max) ? (max) : (target);  \
    _wanted;                                            \
})

FACET_ROW_SEVERITY system_unit_severity(UnitInfo *u) {
    FACET_ROW_SEVERITY severity, max_severity;

    switch(u->UnitLoadState) {
        case UNIT_ERROR:
        case UNIT_BAD_SETTING:
            severity = FACET_ROW_SEVERITY_CRITICAL;
            max_severity = FACET_ROW_SEVERITY_CRITICAL;
            break;

        default:
            severity = FACET_ROW_SEVERITY_WARNING;
            max_severity = FACET_ROW_SEVERITY_CRITICAL;
            break;

        case UNIT_NOT_FOUND:
            severity = FACET_ROW_SEVERITY_NOTICE;
            max_severity = FACET_ROW_SEVERITY_NOTICE;
            break;

        case UNIT_LOADED:
            severity = FACET_ROW_SEVERITY_NORMAL;
            max_severity = FACET_ROW_SEVERITY_CRITICAL;
            break;

        case UNIT_MERGED:
        case UNIT_MASKED:
        case UNIT_STUB:
            severity = FACET_ROW_SEVERITY_DEBUG;
            max_severity = FACET_ROW_SEVERITY_DEBUG;
            break;
    }

    switch(u->UnitActiveState) {
        case UNIT_FAILED:
            severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
            break;

        default:
        case UNIT_RELOADING:
        case UNIT_ACTIVATING:
        case UNIT_DEACTIVATING:
            severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
            break;

        case UNIT_MAINTENANCE:
            severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
            break;

        case UNIT_ACTIVE:
            break;

        case UNIT_INACTIVE:
            severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
            break;
    }

    switch(u->UnitType) {
        case UNIT_SERVICE:
            switch(u->ServiceState) {
                case SERVICE_FAILED:
                case SERVICE_FAILED_BEFORE_AUTO_RESTART:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case SERVICE_STOP:
                case SERVICE_STOP_WATCHDOG:
                case SERVICE_STOP_SIGTERM:
                case SERVICE_STOP_SIGKILL:
                case SERVICE_STOP_POST:
                case SERVICE_FINAL_WATCHDOG:
                case SERVICE_FINAL_SIGTERM:
                case SERVICE_FINAL_SIGKILL:
                case SERVICE_AUTO_RESTART:
                case SERVICE_AUTO_RESTART_QUEUED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case SERVICE_CONDITION:
                case SERVICE_START_PRE:
                case SERVICE_START:
                case SERVICE_START_POST:
                case SERVICE_RELOAD:
                case SERVICE_RELOAD_SIGNAL:
                case SERVICE_RELOAD_NOTIFY:
                case SERVICE_DEAD_RESOURCES_PINNED:
                case SERVICE_CLEANING:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case SERVICE_EXITED:
                case SERVICE_RUNNING:
                    break;

                case SERVICE_DEAD:
                case SERVICE_DEAD_BEFORE_AUTO_RESTART:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_MOUNT:
            switch(u->MountState) {
                case MOUNT_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case MOUNT_REMOUNTING_SIGTERM:
                case MOUNT_REMOUNTING_SIGKILL:
                case MOUNT_UNMOUNTING_SIGTERM:
                case MOUNT_UNMOUNTING_SIGKILL:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case MOUNT_MOUNTING:
                case MOUNT_MOUNTING_DONE:
                case MOUNT_REMOUNTING:
                case MOUNT_UNMOUNTING:
                case MOUNT_CLEANING:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case MOUNT_MOUNTED:
                    break;

                case MOUNT_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_SWAP:
            switch(u->SwapState) {
                case SWAP_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case SWAP_DEACTIVATING_SIGTERM:
                case SWAP_DEACTIVATING_SIGKILL:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case SWAP_ACTIVATING:
                case SWAP_ACTIVATING_DONE:
                case SWAP_DEACTIVATING:
                case SWAP_CLEANING:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case SWAP_ACTIVE:
                    break;

                case SWAP_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_SOCKET:
            switch(u->SocketState) {
                case SOCKET_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case SOCKET_STOP_PRE_SIGTERM:
                case SOCKET_STOP_PRE_SIGKILL:
                case SOCKET_FINAL_SIGTERM:
                case SOCKET_FINAL_SIGKILL:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case SOCKET_START_PRE:
                case SOCKET_START_CHOWN:
                case SOCKET_START_POST:
                case SOCKET_STOP_PRE:
                case SOCKET_STOP_POST:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case SOCKET_RUNNING:
                case SOCKET_LISTENING:
                    break;

                case SOCKET_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_TARGET:
            switch(u->TargetState) {
                default:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case TARGET_ACTIVE:
                    break;

                case TARGET_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_DEVICE:
            switch(u->DeviceState) {
                default:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case DEVICE_TENTATIVE:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case DEVICE_PLUGGED:
                    break;

                case DEVICE_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_AUTOMOUNT:
            switch(u->AutomountState) {
                case AUTOMOUNT_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case AUTOMOUNT_WAITING:
                case AUTOMOUNT_RUNNING:
                    break;

                case AUTOMOUNT_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_TIMER:
            switch(u->TimerState) {
                case TIMER_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case TIMER_ELAPSED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case TIMER_WAITING:
                case TIMER_RUNNING:
                    break;

                case TIMER_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_PATH:
            switch(u->PathState) {
                case PATH_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case PATH_WAITING:
                case PATH_RUNNING:
                    break;

                case PATH_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_SLICE:
            switch(u->SliceState) {
                default:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case SLICE_ACTIVE:
                    break;

                case SLICE_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        case UNIT_SCOPE:
            switch(u->ScopeState) {
                case SCOPE_FAILED:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_CRITICAL);
                    break;

                default:
                case SCOPE_STOP_SIGTERM:
                case SCOPE_STOP_SIGKILL:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
                    break;

                case SCOPE_ABANDONED:
                case SCOPE_START_CHOWN:
                    severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_NOTICE);
                    break;

                case SCOPE_RUNNING:
                    break;

                case SCOPE_DEAD:
                    severity = if_normal(severity, max_severity, FACET_ROW_SEVERITY_DEBUG);
                    break;
            }
            break;

        default:
            severity = if_less(severity, max_severity, FACET_ROW_SEVERITY_WARNING);
            break;
    }

    u->severity = severity;
    return severity;
}

int unit_info_compar(const void *a, const void *b) {
    UnitInfo *u1 = *((UnitInfo **)a);
    UnitInfo *u2 = *((UnitInfo **)b);

    return strcasecmp(u1->id, u2->id);
}

void systemd_units_assign_priority(UnitInfo *base) {
    size_t units = 0, c = 0, prio = 0;
    for(UnitInfo *u = base; u ; u = u->next)
        units++;

    UnitInfo *array[units];
    for(UnitInfo *u = base; u ; u = u->next)
        array[c++] = u;

    qsort(array, units, sizeof(UnitInfo *), unit_info_compar);

    for(c = 0; c < units ; c++) {
        array[c]->prio = prio++;
        system_unit_severity(array[c]);
        systemd_unit_priority(array[c], units);
    }
}

void function_systemd_units(const char *transaction, char *function, int timeout, bool *cancelled) {
    char *words[SYSTEMD_UNITS_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_UNITS_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_UNITS_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, "info") == 0) {
            netdata_systemd_units_function_info(transaction);
            return;
        }
        else if(strcmp(keyword, "help") == 0) {
            netdata_systemd_units_function_help(transaction);
            return;
        }
    }

    UnitInfo *base = systemd_units_get_all();
    systemd_units_assign_priority(base);

    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 10);
    buffer_json_member_add_string(wb, "help", SYSTEMD_UNITS_FUNCTION_DESCRIPTION);
    buffer_json_member_add_array(wb, "data");

    for(UnitInfo *u = base; u ;u = u->next) {
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, u->id);

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "severity", facets_severity_to_string(u->severity));
            }
            buffer_json_object_close(wb);

            buffer_json_add_array_item_string(wb, u->type);
            buffer_json_add_array_item_string(wb, u->description);
            buffer_json_add_array_item_string(wb, u->load_state);
            buffer_json_add_array_item_string(wb, u->active_state);
            buffer_json_add_array_item_string(wb, u->sub_state);
            buffer_json_add_array_item_string(wb, u->following);
            buffer_json_add_array_item_string(wb, u->unit_path);
            buffer_json_add_array_item_uint64(wb, u->job_id);
            buffer_json_add_array_item_string(wb, u->job_type);
            buffer_json_add_array_item_string(wb, u->job_path);
            buffer_json_add_array_item_uint64(wb, u->prio);
            buffer_json_add_array_item_uint64(wb, 1); // count
        }
        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "id", "Unit ID",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(
                wb, field_id++,
                "rowOptions", "rowOptions",
                RRDF_FIELD_TYPE_NONE,
                RRDR_FIELD_VISUAL_ROW_OPTIONS,
                RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_DUMMY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "type", "Unit Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "description", "Unit Description",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "loadState", "Unit Load State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "activeState", "Unit Active State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "subState", "Unit Sub State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "following", "Unit Following",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_WRAP,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "path", "Unit Path",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobId", "Unit Job ID",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobType", "Unit Job Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobPath", "Unit Job Path",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "priority", "Priority",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "count", "Count",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "priority");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "count");
        {
            buffer_json_member_add_string(wb, "name", "count");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "count");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "count");
        buffer_json_add_array_item_string(wb, "activeState");
        buffer_json_array_close(wb);
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "count");
        buffer_json_add_array_item_string(wb, "subState");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "type");
        {
            buffer_json_member_add_string(wb, "name", "Top Down Tree");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "type");
                buffer_json_add_array_item_string(wb, "loadState");
                buffer_json_add_array_item_string(wb, "activeState");
                buffer_json_add_array_item_string(wb, "subState");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "subState");
        {
            buffer_json_member_add_string(wb, "name", "Bottom Up Tree");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "subState");
                buffer_json_add_array_item_string(wb, "activeState");
                buffer_json_add_array_item_string(wb, "loadState");
                buffer_json_add_array_item_string(wb, "type");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
    systemd_units_free_all(base);
}

#endif // ENABLE_SYSTEMD_DBUS
