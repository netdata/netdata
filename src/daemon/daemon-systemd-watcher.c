// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "daemon-systemd-watcher.h"
#include "daemon-service.h"
#include "daemon-shutdown.h"

#ifdef ENABLE_SYSTEMD_DBUS

#include <systemd/sd-bus.h>

/* Callback function to handle the PrepareForShutdown signal.
 * The signal sends a boolean: true indicates that shutdown is starting,
 * false indicates that a previously initiated shutdown was canceled.
 */
static int shutdown_event_handler(sd_bus_message *m, void *userdata __maybe_unused, sd_bus_error *ret_error __maybe_unused) {
    int shutdown;
    int r = sd_bus_message_read(m, "b", &shutdown);
    if (r < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SYSTEMD DBUS: Failed to parse shutdown message: %s",
               strerror(-r));
        return r;
    }

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "SYSTEMD DBUS: Received PrepareForShutdown signal: shutdown=%s",
           shutdown ? "true" : "false");

    if(shutdown)
        netdata_cleanup_and_exit(EXIT_REASON_SYSTEM_SHUTDOWN, NULL, NULL, NULL);

    return 0;
}

/* Callback function to handle the PrepareForSleep signal.
 * The signal sends a boolean: true indicates that the system is preparing to suspend,
 * false indicates that a previous suspend was canceled (i.e. resuming).
 */
static int suspend_event_handler(sd_bus_message *m, void *userdata __maybe_unused, sd_bus_error *ret_error __maybe_unused) {
    int suspend;
    int r = sd_bus_message_read(m, "b", &suspend);
    if (r < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SYSTEMD DBUS: Failed to parse suspend message: %s",
               strerror(-r));
        return r;
    }

    nd_log(NDLS_DAEMON, NDLP_NOTICE,
           "SYSTEMD DBUS: Received PrepareForSleep signal: suspend=%s\n",
           suspend ? "true (suspending)" : "false (resuming)");

    // Here you can trigger your suspend/resume logic.
    return 0;
}

/* Function that sets up the sd-bus listener for shutdown and suspend events.
 * This function blocks in a loop processing bus events.
 */
static void listen_for_systemd_dbus_events(void) {
    sd_bus *bus = NULL;
    sd_bus_slot *shutdown_slot = NULL;
    sd_bus_slot *suspend_slot = NULL;
    int r;

    // Connect to the system bus.
    r = sd_bus_open_system(&bus);
    if (r < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SYSTEMD DBUS: Failed to connect to system bus: %s",
               strerror(-r));
        goto finish;
    }

    // Add a match rule for the PrepareForShutdown signal on the login1 manager.
    r = sd_bus_add_match(
        bus,
        &shutdown_slot,
        "type='signal',"
        "sender='org.freedesktop.login1',"
        "interface='org.freedesktop.login1.Manager',"
        "member='PrepareForShutdown'",
        shutdown_event_handler,
        NULL);
    if (r < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SYSTEMD DBUS: Failed to add signal match for shutdown: %s",
               strerror(-r));
        goto finish;
    }

    // Add a match rule for the PrepareForSleep signal on the login1 manager.
    r = sd_bus_add_match(
        bus,
        &suspend_slot,
        "type='signal',"
        "sender='org.freedesktop.login1',"
        "interface='org.freedesktop.login1.Manager',"
        "member='PrepareForSleep'",
        suspend_event_handler,
        NULL);
    if (r < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SYSTEMD DBUS: Failed to add signal match for suspend: %s",
               strerror(-r));
        goto finish;
    }

    // Process incoming D-Bus messages.
    while (service_running(SERVICE_SYSTEMD) && bus != NULL) {
        // Process any pending messages.
        r = sd_bus_process(bus, NULL);
        if (r < 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "SYSTEMD DBUS: Failed to process bus: %s",
                   strerror(-r));
            goto finish;
        }
        if (r > 0) // Message was processed; check for more.
            continue;

        // Wait for the next signal.
        do {
            r = sd_bus_wait(bus, USEC_PER_SEC);
        } while((r == 0 || r == -EINTR) && service_running(SERVICE_SYSTEMD));

        if (r < 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "SYSTEMD DBUS: Failed to wait on bus: %s", strerror(-r));
            break;
        }
    }

finish:
    sd_bus_slot_unref(shutdown_slot);
    sd_bus_slot_unref(suspend_slot);
    sd_bus_unref(bus);
}

#endif

void *systemd_watcher_thread(void *arg) {
    struct netdata_static_thread *static_thread = arg;

    service_register(SERVICE_THREAD_TYPE_NETDATA, NULL, NULL, NULL, false);

#ifdef ENABLE_SYSTEMD_DBUS
    listen_for_systemd_dbus_events();
#endif

    service_exits();
    worker_unregister();
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}
