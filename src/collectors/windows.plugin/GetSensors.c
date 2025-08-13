// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include <sensorsapi.h>
#include <sensors.h>
#include <propidl.h>

static SensorManager *pSensorManager = NULL;
static ND_THREAD *sensors_thread_update = NULL;

static void netdata_sensors_monitor(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = UPDATE_EVERY_MIN;

    while (service_running(SERVICE_COLLECTORS)) {
        (void) heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;
    }
}

static int initialize(int update_every)
{
    HRESULT hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Collector cannot initialize COM interface.");
        return -1;
    }

    hr = CoCreateInstance(
        &CLSID_SensorManager, NULL, CLSCTX_INPROC_SERVER, &IID_ISensorManager, (void **)&pSensorManager);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Collector cannot initialize sensor API.");
        CoUninitialize();
        return -1;
    }

    sensors_thread_update =
        nd_thread_create("sensors_upd", NETDATA_THREAD_OPTION_DEFAULT, netdata_sensors_monitor, &update_every);

    return 0;
}

int do_GetSensors(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        if (likely(initialize(update_every))) {
            return -1;
        }

        initialized = true;
    }
    return 0;
}

void do_Sensors_cleanup()
{
    if (nd_thread_join(sensors_thread_update))
        nd_log_daemon(NDLP_ERR, "Failed to join sensors thread update");
}