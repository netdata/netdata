// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"

#include <windows.h>
#include <wchar.h>

#include <sensorsapi.h>
#include <sensors.h>
#include <propidl.h>

static ISensorManager *pSensorManager = NULL;
static ND_THREAD *sensors_thread_update = NULL;

#define NETDATA_WIN_SENSOR_STATES (6)

enum netdata_win_sensor_monitored {
    NETDATA_WIN_SENSOR_CELSIUS,
    NETDATA_WIN_SENSOR_POWER_WATTS,
    NETDATA_WIN_SENSOR_CURRENT_AMPS,
    NETDATA_WIN_SENSOR_RELATIVITY_HUMIDITY,
    NETDATA_WIN_SENSOR_LIGHT_LEVEL,
    NETDATA_WIN_SENSOR_DATA_TYPE_LIGHT_TEMPERATURE,
    NETDATA_WIN_SENSOR_VOLTAGE,
    NETDATA_WIN_SENSOR_RESISTENCE,
    NETDATA_WIN_SENSOR_ATMOSPHERE_PRESSURE,
    NETDATA_WIN_SENSOR_LATITUDE_DEGREES,
    NETDATA_WIN_SENSOR_LONGITUDE_DEGREES,
    NETDATA_WIN_SENSOR_FORCE_NEWTONS,
    NETDATA_WIN_SENSOR_GAUGE_PRESSURE,
    NETDATA_WIN_SENSOR_TYPE_DISTANCE_X,
    NETDATA_WIN_SENSOR_TYPE_DISTANCE_Y,
    NETDATA_WIN_SENSOR_TYPE_DISTANCE_Z
};

REFPROPERTYKEY sensor_keys[] = {
        &SENSOR_DATA_TYPE_TEMPERATURE_CELSIUS,
        &SENSOR_DATA_TYPE_ELECTRICAL_POWER_WATTS,
        &SENSOR_DATA_TYPE_CURRENT_AMPS,
        &SENSOR_DATA_TYPE_RELATIVE_HUMIDITY_PERCENT,
        &SENSOR_DATA_TYPE_LIGHT_LEVEL_LUX,
        &SENSOR_DATA_TYPE_LIGHT_TEMPERATURE_KELVIN,
        &SENSOR_DATA_TYPE_VOLTAGE_VOLTS,
        &SENSOR_DATA_TYPE_RESISTANCE_OHMS,
        &SENSOR_DATA_TYPE_ATMOSPHERIC_PRESSURE_BAR,
        &SENSOR_DATA_TYPE_LATITUDE_DEGREES,
        &SENSOR_DATA_TYPE_LONGITUDE_DEGREES,
        &SENSOR_DATA_TYPE_FORCE_NEWTONS,
        &SENSOR_DATA_TYPE_GAUGE_PRESSURE_PASCAL,
        &SENSOR_DATA_TYPE_DISTANCE_X_METERS,
        &SENSOR_DATA_TYPE_DISTANCE_Y_METERS,
        &SENSOR_DATA_TYPE_DISTANCE_Z_METERS,

        NULL};


static struct win_sensor_config {
    const char *title;
    const char *units;
    const char *context;
    const char *family;

    int priority;
} configs[] = {
        {
            .title = "Sensor Temperature",
            .units = "Cel",
            .context = "system.hw.sensor.temperature.input",
            .family = "Temperature",
            .priority = 70000,
        },
        {
            .title = "Sensor Power",
            .units = "W",
            .context = "system.hw.sensor.power.input",
            .family = "Power",
            .priority = 70006,
        },
        {
            .title = "Sensor Current",
            .units = "A",
            .context = "system.hw.sensor.current.input",
            .family = "Current",
            .priority = 70003,
        },
        {
            .title = "Sensor Humidity",
            .units = "%",
            .context = "system.hw.sensor.humidity.input",
            .family = "Humidity",
            .priority = 70004,
        },
        {
            .title = "Ambient light level",
            .units = "lx",
            .context = "system.hw.sensor.lux.input",
            .family = "illuminance",
            .priority = 70010,
        },
        {
            .title = "Color temperature of light",
            .units = "Cel",
            .context = "system.hw.sensor.color.input",
            .family = "Temperature",
            .priority = 70011,
        },
        {
            .title = "Electrical potential.",
            .units = "V",
            .context = "system.hw.sensor.voltage.input",
            .family = "Potential",
            .priority = 70012,
        },
        {
            .title = "Electrical resistence.",
            .units = "Ohms",
            .context = "system.hw.sensor.resistence.input",
            .family = "Resistence",
            .priority = 70013,
        },
        {
            .title = "Ambient atmospheric pressure",
            .units = "Pa",
            .context = "system.hw.sensor.pressure.input",
            .family = "Pressure",
            .priority = 70014,
        },
        {
            .title = "Geographic latitude",
            .units = "Degrees",
            .context = "system.hw.sensor.latitude.input",
            .family = "Location",
            .priority = 70015,
        },
        {
            .title = "Geographic longitude",
            .units = "Degrees",
            .context = "system.hw.sensor.longitude.input",
            .family = "Location",
            .priority = 70016,
        }
};

struct sensor_data {
    bool initialized;
    bool first_time;
    bool enabled;
    enum netdata_win_sensor_monitored sensor_data_type;
    struct win_sensor_config *config;

    const char *type;
    const char *category;
    const char *name;
    const char *manufacturer;
    const char *model;

    SensorState current_state;

    RRDSET *st_sensor_state;
    RRDDIM *rd_sensor_state[NETDATA_WIN_SENSOR_STATES];

    RRDSET *st_sensor_data;
    RRDDIM *rd_sensor_data;

    collected_number current_data_value;
    NETDATA_DOUBLE mult_factor;
    NETDATA_DOUBLE div_factor;
    NETDATA_DOUBLE add_factor;
};

DICTIONARY *sensors;

// Microsoft appends additional data
#define ADDTIONAL_UUID_STR_LEN (UUID_STR_LEN + 8)

static void netdata_clsid_to_char(char *output, const GUID *pguid)
{
    LPWSTR wguid = NULL;
    if (SUCCEEDED(StringFromCLSID(pguid, &wguid)) && wguid) {
        size_t len = wcslen(wguid);
        wcstombs(output, wguid, len);
        CoTaskMemFree(wguid);
    }
}

static inline char *netdata_convert_guid_to_string(HRESULT hr, GUID *value) {
    if (SUCCEEDED(hr)) {
        char cguid[ADDTIONAL_UUID_STR_LEN];
        netdata_clsid_to_char(cguid, value);
        return strdupz(cguid);
    }
    return NULL;
}

static inline void netdata_fill_sensor_type(struct sensor_data *sd, ISensor *pSensor)
{
    GUID type = {0};
    HRESULT hr = pSensor->lpVtbl->GetType(pSensor, &type);
    sd->type =  netdata_convert_guid_to_string(hr, &type);
}

static inline void netdata_fill_sensor_category(struct sensor_data *sd, ISensor *pSensor)
{
    GUID category = {0};
    HRESULT hr = pSensor->lpVtbl->GetCategory(pSensor, &category);
    sd->category =  netdata_convert_guid_to_string(hr, &category);
}

static inline char *netdata_pvar_to_char(const PROPERTYKEY *key, ISensor *pSensor)
{
    PROPVARIANT pv;
    PropVariantInit(&pv);
    HRESULT hr = pSensor->lpVtbl->GetProperty(pSensor, key, &pv);
    if (SUCCEEDED(hr) && pv.vt == VT_LPWSTR) {
        char value[8192];
        size_t len = wcslen(pv.pwszVal);
        wcstombs(value, pv.pwszVal, len);
        PropVariantClear(&pv);
        return strdupz(value);
    }
    PropVariantClear(&pv);
    return NULL;
}

static void netdata_initialize_sensor_dict(struct sensor_data *sd, ISensor *pSensor)
{
    netdata_fill_sensor_type(sd, pSensor);
    netdata_fill_sensor_category(sd, pSensor);
    sd->name = netdata_pvar_to_char(&SENSOR_PROPERTY_FRIENDLY_NAME, pSensor);
    sd->model = netdata_pvar_to_char(&SENSOR_PROPERTY_MODEL, pSensor);
    sd->manufacturer = netdata_pvar_to_char(&SENSOR_PROPERTY_MANUFACTURER, pSensor);
}

static int
netdata_collect_sensor_data(struct sensor_data *sd, ISensor *pSensor, REFPROPERTYKEY key)
{
    ISensorDataReport *pReport = NULL;
    PROPVARIANT pv = {};
    HRESULT hr;

    int defined = 0;
    hr = pSensor->lpVtbl->GetData(pSensor, &pReport);
    if (SUCCEEDED(hr) && pReport) {
        PropVariantInit(&pv);
        hr = pReport->lpVtbl->GetSensorValue(pReport, key, &pv);
        if (SUCCEEDED(hr) && (pv.vt == VT_R4 || pv.vt == VT_R8 || pv.vt == VT_UI4)) {
            switch (pv.vt) {
                case VT_UI4:
                    sd->current_data_value = (collected_number)(pv.ulVal * 100);
                    break;
                case VT_R4:
                    sd->current_data_value = (collected_number)(pv.fltVal * sd->div_factor);
                    break;
                case VT_R8:
                    sd->current_data_value = (collected_number)(pv.dblVal * sd->div_factor);
                    break;
            }
            defined = 1;
        }
        PropVariantClear(&pv);
        pReport->lpVtbl->Release(pReport);
    }

    return defined;
}

static void netdata_sensors_get_data(struct sensor_data *sd, ISensor *pSensor)
{
    for (int i = 0; sensor_keys[i]; i++) {
        if (netdata_collect_sensor_data(sd, pSensor, sensor_keys[i])) {
            sd->sensor_data_type = i;
            sd->config = &configs[i];
            sd->enabled = true;
            if (i == NETDATA_WIN_SENSOR_DATA_TYPE_LIGHT_TEMPERATURE) {
                sd->div_factor = 100.0;
                sd->add_factor = -27315.0; // 273.15 * 100.0
            }
            break;
        }
    }

    sd->first_time = false;
}

static void netdata_get_sensors()
{
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

        struct sensor_data *sd = dictionary_set(sensors, thread_values, NULL, sizeof(*sd));

        if (unlikely(!sd->initialized)) {
            netdata_initialize_sensor_dict(sd, pSensor);
            sd->initialized = true;
        }

        sd->current_state = SENSOR_STATE_MIN;
        (void)pSensor->lpVtbl->GetState(pSensor, &sd->current_state);

        if (likely(sd->first_time))
            netdata_sensors_get_data(sd, pSensor);
        else if (likely(sd->enabled))
            netdata_collect_sensor_data(sd, pSensor, sensor_keys[sd->sensor_data_type]);

        pSensor->lpVtbl->Release(pSensor);
    }

    pSensorCollection->lpVtbl->Release(pSensorCollection);
}

static void netdata_sensors_monitor(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (service_running(SERVICE_COLLECTORS)) {
        (void)heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        netdata_get_sensors();
    }
}

void dict_sensor_insert(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct sensor_data *sd = value;

    sd->first_time = true;
    sd->sensor_data_type = 0;
    sd->config = NULL;
    sd->mult_factor = 1.0;
    sd->div_factor = 100.0;
    sd->add_factor = 0.0;
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

static void sensors_states_chart(struct sensor_data *sd, int update_every)
{
    if (!sd->st_sensor_state) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "%s_state", sd->name);
        netdata_fix_chart_name(id);
        sd->st_sensor_state = rrdset_create_localhost(
            "sensors",
            id,
            NULL,
            "sensors",
            "system.hw.sensor.state",
            "Current sensor state.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "GetSensors",
            70010,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(sd->st_sensor_state->rrdlabels, "name", sd->name, RRDLABEL_SRC_AUTO);
        rrdlabels_add(sd->st_sensor_state->rrdlabels, "manufacturer", sd->manufacturer, RRDLABEL_SRC_AUTO);
        rrdlabels_add(sd->st_sensor_state->rrdlabels, "model", sd->model, RRDLABEL_SRC_AUTO);

        sd->rd_sensor_state[0] = rrddim_add(sd->st_sensor_state, "ready", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sd->rd_sensor_state[1] = rrddim_add(sd->st_sensor_state, "not_available", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sd->rd_sensor_state[2] = rrddim_add(sd->st_sensor_state, "no_data", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sd->rd_sensor_state[3] = rrddim_add(sd->st_sensor_state, "initializing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sd->rd_sensor_state[4] = rrddim_add(sd->st_sensor_state, "access_denied", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        sd->rd_sensor_state[5] = rrddim_add(sd->st_sensor_state, "error", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
}

static void sensors_state_chart_loop(struct sensor_data *sd, int update_every)
{
    sensors_states_chart(sd, update_every);
    collected_number set_value = (collected_number)sd->current_state;
    for (collected_number i = 0; i < NETDATA_WIN_SENSOR_STATES; i++) {
        rrddim_set_by_pointer(sd->st_sensor_state, sd->rd_sensor_state[i], i == set_value);
    }
    rrdset_done(sd->st_sensor_state);
}

static void sensors_data_chart(struct sensor_data *sd, int update_every)
{
    if (!sd->st_sensor_data) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "sensors.%s_input", sd->name);
        netdata_fix_chart_name(id);
        sd->st_sensor_data = rrdset_create_localhost(
                "sensors",
                id,
                NULL,
                sd->config->family,
                sd->config->context,
                sd->config->title,
                sd->config->units,
                PLUGIN_WINDOWS_NAME,
                "GetSensors",
                sd->config->priority,
                update_every,
                RRDSET_TYPE_LINE);

        rrdlabels_add(sd->st_sensor_data->rrdlabels, "name", sd->name, RRDLABEL_SRC_AUTO);
        rrdlabels_add(sd->st_sensor_data->rrdlabels, "manufacturer", sd->manufacturer, RRDLABEL_SRC_AUTO);
        rrdlabels_add(sd->st_sensor_data->rrdlabels, "model", sd->model, RRDLABEL_SRC_AUTO);

        sd->rd_sensor_data = rrddim_add(sd->st_sensor_data, "input", NULL, (collected_number )sd->mult_factor, (collected_number )sd->div_factor, RRD_ALGORITHM_ABSOLUTE);
    }

    collected_number value = sd->current_data_value + (collected_number )sd->add_factor;
    rrddim_set_by_pointer(sd->st_sensor_data, sd->rd_sensor_data, value);
    rrdset_done(sd->st_sensor_data);
}

int dict_sensors_charts_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    int *update_every = data;
    struct sensor_data *sd = value;

    if (unlikely(!sd->name))
        return 1;

    sensors_state_chart_loop(sd, *update_every);

    if (unlikely(!sd->enabled))
        return 1;

    sensors_data_chart(sd, *update_every);

    return 1;
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

    dictionary_sorted_walkthrough_read(sensors, dict_sensors_charts_cb, &update_every);
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