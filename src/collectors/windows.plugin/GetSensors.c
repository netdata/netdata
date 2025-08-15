// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"

#include <windows.h>

#include <sensorsapi.h>
#include <sensors.h>
#include <propidl.h>

static ISensorManager *pSensorManager = NULL;
static ND_THREAD *sensors_thread_update = NULL;

struct sensor_data {
    bool initialized;
    const char *type;
    const char *category;
    const char *name;
    const char *manufacturer;
    const char *model;

    SensorState current_state;
};

DICTIONARY *sensors;

// Microsoft appends additional data
#define ADDTIONAL_UUID_STR_LEN (UUID_STR_LEN+8)

static void netdata_clsid_to_char(char *output, const GUID *pguid) {
    LPWSTR wguid = NULL;
    if (SUCCEEDED(StringFromCLSID(pguid, &wguid)) && wguid) {
        size_t len = wcslen(wguid);
        wcstombs(output, wguid, len);
        CoTaskMemFree(wguid);
    }
}

static inline void netdata_fill_sensor_type(struct sensor_data *s, ISensor *pSensor) {
    GUID type = {0};
    char cguid[ADDTIONAL_UUID_STR_LEN];
    HRESULT hr = pSensor->lpVtbl->GetType(pSensor, &type);
    if (SUCCEEDED(hr)) {
        netdata_clsid_to_char(cguid, &type);
        s->type = strdupz(cguid);
    }
}

static inline void netdata_fill_sensor_category(struct sensor_data *s, ISensor *pSensor) {
    GUID category = {0};
    char cguid[ADDTIONAL_UUID_STR_LEN];
    HRESULT hr = pSensor->lpVtbl->GetCategory(pSensor, &category);
    if (SUCCEEDED(hr)) {
        netdata_clsid_to_char(cguid, &category);
        s->category = strdupz(cguid);
    }
}

static inline char *netdata_pvar_to_char(HRESULT hr, PROPVARIANT *pv) {
    if (SUCCEEDED(hr) && pv->vt == VT_LPWSTR) {
        char value[8192];
        size_t len = wcslen(pv->pwszVal);
        wcstombs(value, pv->pwszVal, len);
    }
    return NULL;
}

static inline void netdata_fill_sensor_name(struct sensor_data *s, ISensor *pSensor) {
    PROPVARIANT pv;
    PropVariantInit(&pv);
    HRESULT hr = pSensor->lpVtbl->GetProperty(pSensor, &SENSOR_PROPERTY_FRIENDLY_NAME, &pv);
    s->name = netdata_pvar_to_char(hr, &pv);
    PropVariantClear(&pv);
}

static inline void netdata_fill_sensor_model(struct sensor_data *s, ISensor *pSensor) {
    PROPVARIANT pv;
    PropVariantInit(&pv);
    HRESULT hr = pSensor->lpVtbl->GetProperty(pSensor, &SENSOR_PROPERTY_MODEL, &pv);
    s->model = netdata_pvar_to_char(hr, &pv);
    PropVariantClear(&pv);
}

static inline void netdata_fill_sensor_manufacturer(struct sensor_data *s, ISensor *pSensor) {
    PROPVARIANT pv;
    PropVariantInit(&pv);
    HRESULT hr = pSensor->lpVtbl->GetProperty(pSensor, &SENSOR_PROPERTY_MANUFACTURER, &pv);
    s->manufacturer = netdata_pvar_to_char(hr, &pv);
    PropVariantClear(&pv);
}

static void netdata_initialize_sensor_dict(struct sensor_data *s, ISensor *pSensor) {
    netdata_fill_sensor_type(s, pSensor);
    netdata_fill_sensor_category(s, pSensor);
    netdata_fill_sensor_name(s, pSensor);
    netdata_fill_sensor_name(s, pSensor);
    netdata_fill_sensor_manufacturer(s, pSensor);
}

static void netdata_get_sensors() {
    ISensorCollection *pSensorCollection = NULL;
    HRESULT hr = pSensorManager->lpVtbl->GetSensorsByCategory(pSensorManager, &SENSOR_CATEGORY_ALL, &pSensorCollection);
    if (FAILED(hr)) {
        return;
    }

    ULONG count = 0;
    hr = pSensorCollection->lpVtbl->GetCount(pSensorCollection, &count);
    if (FAILED(hr)) {
        return;
    }

    // the same size of windows_shared_buffer
    char thread_values[8192];
    for (ULONG i = 0; i < count; ++i) {
        ISensor *pSensor = NULL;
        hr = pSensorCollection->lpVtbl->GetAt(pSensorCollection, i, &pSensor);
        if (FAILED(hr) || !pSensor) {
            continue;
        }

        GUID id = {0};
        hr = pSensor->lpVtbl->GetID(pSensor, &id);
        if (FAILED(hr)) {
            continue;
        }
        netdata_clsid_to_char(thread_values, &id);

        struct sensor_data *s = dictionary_set(sensors, thread_values, NULL, sizeof(*s));

        if (unlikely(!s->initialized)) {
            netdata_initialize_sensor_dict(s, pSensor);
            s->initialized = true;
        }

        s->current_state = SENSOR_STATE_MIN;
        (void)pSensor->lpVtbl->GetState(pSensor, &s->current_state);

        pSensor->lpVtbl->Release(pSensor);
    }

    pSensorCollection->lpVtbl->Release(pSensorCollection);
}

static void netdata_sensors_monitor(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = UPDATE_EVERY_MIN;

    while (service_running(SERVICE_COLLECTORS)) {
        (void) heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        netdata_get_sensors();
    }
}

void dict_sensor_insert(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    ;
}

static int initialize(int update_every)
{
    // This is an internal plugin, if we initialize these two times, collector will fail. To avoid this
    // we call InitializeWMI to verify COM interface was already initialized.
    HRESULT hr = InitializeWMI();
    if (hr != S_OK) {
        hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
        if (FAILED(hr)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Collector cannot initialize COM interface.");
            return -1;
        }
    }

    hr = CoCreateInstance(
        &CLSID_SensorManager, NULL, CLSCTX_INPROC_SERVER, &IID_ISensorManager, (void **)&pSensorManager);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Collector cannot initialize sensor API.");
        CoUninitialize();
        return -1;
    }

    sensors = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct sensor_data));
    dictionary_register_insert_callback(sensors, dict_sensor_insert, NULL);

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

   if (pSensorManager) {
       pSensorManager->lpVtbl->Release(pSensorManager);

       CoUninitialize();
   }
}