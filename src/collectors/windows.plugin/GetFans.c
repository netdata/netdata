// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"

#define _COMMON_PLUGIN_NAME PLUGIN_WINDOWS_NAME
#define _COMMON_PLUGIN_MODULE_NAME "GetFans"

struct win_fan {
    char *id;
    char *name;
    char *device_id;
    char *status;

    bool seen;
    bool desired_speed_valid;
    uint64_t desired_speed;

    bool active_cooling_valid;
    bool active_cooling;
    bool variable_speed_valid;
    bool variable_speed;
    bool online_valid;
    bool online;

    RRDSET *st_speed;
    RRDDIM *rd_speed;
    RRDSET *st_state;
    RRDDIM *rd_active_cooling;
    RRDDIM *rd_variable_speed;
    RRDDIM *rd_online;

    struct win_fan *next;
};

static struct win_fan *fans_root = NULL;

static void wmi_bstr_to_char(BSTR value, char *dst, size_t dst_size)
{
    if (!dst || !dst_size)
        return;

    dst[0] = '\0';
    if (!value)
        return;

    wcstombs(dst, value, dst_size - 1);
    dst[dst_size - 1] = '\0';
}

static bool variant_to_bool(VARIANT *vt, bool *value)
{
    if (!vt || !value || vt->vt != VT_BOOL)
        return false;

    *value = vt->boolVal != VARIANT_FALSE;
    return true;
}

static bool variant_to_uint64(VARIANT *vt, uint64_t *value)
{
    if (!vt || !value)
        return false;

    switch (vt->vt) {
        case VT_UI1:
            *value = vt->bVal;
            return true;
        case VT_I2:
            *value = vt->iVal < 0 ? 0 : (uint64_t)vt->iVal;
            return true;
        case VT_UI2:
            *value = vt->uiVal;
            return true;
        case VT_I4:
        case VT_INT:
            *value = vt->lVal < 0 ? 0 : (uint64_t)vt->lVal;
            return true;
        case VT_UI4:
        case VT_UINT:
            *value = vt->ulVal;
            return true;
        case VT_I8:
            *value = vt->llVal < 0 ? 0 : (uint64_t)vt->llVal;
            return true;
        case VT_UI8:
            *value = vt->ullVal;
            return true;
        case VT_R4:
            *value = vt->fltVal < 0 ? 0 : (uint64_t)vt->fltVal;
            return true;
        case VT_R8:
            *value = vt->dblVal < 0 ? 0 : (uint64_t)vt->dblVal;
            return true;
        case VT_BSTR: {
            char buf[64];
            wmi_bstr_to_char(vt->bstrVal, buf, sizeof(buf));
            if (!*buf)
                return false;
            *value = strtoull(buf, NULL, 10);
            return true;
        }
        default:
            return false;
    }
}

static void obsolete_fan_charts(struct win_fan *fan)
{
    if (fan->st_speed)
        rrdset_is_obsolete___safe_from_collector_thread(fan->st_speed);
    if (fan->st_state)
        rrdset_is_obsolete___safe_from_collector_thread(fan->st_state);

    fan->st_speed = NULL;
    fan->rd_speed = NULL;
    fan->st_state = NULL;
    fan->rd_active_cooling = NULL;
    fan->rd_variable_speed = NULL;
    fan->rd_online = NULL;
}

static void free_fan(struct win_fan *fan)
{
    if (!fan)
        return;

    obsolete_fan_charts(fan);
    freez(fan->id);
    freez(fan->name);
    freez(fan->device_id);
    freez(fan->status);
    freez(fan);
}

static struct win_fan *get_or_create_fan(const char *id)
{
    for (struct win_fan *fan = fans_root; fan; fan = fan->next) {
        if (fan->id && strcmp(fan->id, id) == 0)
            return fan;
    }

    struct win_fan *fan = callocz(1, sizeof(*fan));
    fan->id = strdupz(id);
    fan->next = fans_root;
    fans_root = fan;

    return fan;
}

static void update_fan_string(char **dst, const char *src)
{
    freez(*dst);
    *dst = (src && *src) ? strdupz(src) : NULL;
}

static void fan_labels(RRDSET *st, struct win_fan *fan)
{
    rrdlabels_add(st->rrdlabels, "fan", fan->name ? fan->name : fan->id, RRDLABEL_SRC_AUTO);

    if (fan->device_id)
        rrdlabels_add(st->rrdlabels, "device_id", fan->device_id, RRDLABEL_SRC_AUTO);
}

static void update_fan_charts(struct win_fan *fan, int update_every)
{
    if (fan->desired_speed_valid) {
        if (!fan->st_speed) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "fan_%s_requested_speed", fan->id);
            netdata_fix_chart_name(id);

            fan->st_speed = rrdset_create_localhost(
                "system",
                id,
                NULL,
                "fans",
                "system.fan_requested_speed",
                "Fan requested speed",
                "RPM",
                PLUGIN_WINDOWS_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_SENSORS + 30,
                update_every,
                RRDSET_TYPE_LINE);

            fan->rd_speed = rrddim_add(fan->st_speed, "requested", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            fan_labels(fan->st_speed, fan);
        }

        rrddim_set_by_pointer(fan->st_speed, fan->rd_speed, (collected_number)fan->desired_speed);
        rrdset_done(fan->st_speed);
    } else if (fan->st_speed) {
        rrdset_is_obsolete___safe_from_collector_thread(fan->st_speed);
        fan->st_speed = NULL;
        fan->rd_speed = NULL;
    }

    if (fan->active_cooling_valid || fan->variable_speed_valid || fan->online_valid) {
        if (!fan->st_state) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "fan_%s_state", fan->id);
            netdata_fix_chart_name(id);

            fan->st_state = rrdset_create_localhost(
                "system",
                id,
                NULL,
                "fans",
                "system.fan_state",
                "Fan state",
                "state",
                PLUGIN_WINDOWS_NAME,
                _COMMON_PLUGIN_MODULE_NAME,
                NETDATA_CHART_PRIO_SENSORS + 31,
                update_every,
                RRDSET_TYPE_LINE);

            fan->rd_active_cooling =
                rrddim_add(fan->st_state, "active_cooling", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            fan->rd_variable_speed =
                rrddim_add(fan->st_state, "variable_speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            fan->rd_online = rrddim_add(fan->st_state, "online", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            fan_labels(fan->st_state, fan);
        }

        rrddim_set_by_pointer(fan->st_state, fan->rd_active_cooling, fan->active_cooling ? 1 : 0);
        rrddim_set_by_pointer(fan->st_state, fan->rd_variable_speed, fan->variable_speed ? 1 : 0);
        rrddim_set_by_pointer(fan->st_state, fan->rd_online, fan->online ? 1 : 0);
        rrdset_done(fan->st_state);
    }
}

static bool get_wmi_string(IWbemClassObject *obj, LPCWSTR prop, char *dst, size_t dst_size)
{
    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
    if (FAILED(hr) || vtProp.vt != VT_BSTR) {
        VariantClear(&vtProp);
        return false;
    }

    wmi_bstr_to_char(vtProp.bstrVal, dst, dst_size);
    VariantClear(&vtProp);
    return *dst != '\0';
}

static bool get_wmi_uint64(IWbemClassObject *obj, LPCWSTR prop, uint64_t *value)
{
    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
    if (FAILED(hr)) {
        VariantClear(&vtProp);
        return false;
    }

    bool ok = variant_to_uint64(&vtProp, value);
    VariantClear(&vtProp);
    return ok;
}

static bool get_wmi_bool(IWbemClassObject *obj, LPCWSTR prop, bool *value)
{
    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
    if (FAILED(hr)) {
        VariantClear(&vtProp);
        return false;
    }

    bool ok = variant_to_bool(&vtProp, value);
    VariantClear(&vtProp);
    return ok;
}

void do_GetFans_cleanup(void)
{
    while (fans_root) {
        struct win_fan *fan = fans_root;
        fans_root = fan->next;
        free_fan(fan);
    }
}

int do_GetFans(int update_every, usec_t dt __maybe_unused)
{
    if (InitializeWMI() != S_OK)
        return -1;

    for (struct win_fan *fan = fans_root; fan; fan = fan->next)
        fan->seen = false;

    IEnumWbemClassObject *pEnumerator = NULL;
    BSTR query = SysAllocString(
        L"SELECT DeviceID, Name, DesiredSpeed, ActiveCooling, Availability, Status, VariableSpeed FROM Win32_Fan");
    BSTR wql = SysAllocString(L"WQL");
    HRESULT hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
        nd_wmi.pSvc, wql, query, WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY, NULL, &pEnumerator);
    SysFreeString(query);
    SysFreeString(wql);

    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetFans() WMI query failed. Error code = 0x%X", hr);
        return -1;
    }

    ULONG returned = 0;
    size_t generated_id = 0;
    bool collected = false;

    for (;;) {
        IWbemClassObject *obj = NULL;
        hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &obj, &returned);
        if (FAILED(hr) || returned == 0)
            break;

        char device_id[256] = "";
        char name[256] = "";
        char status[64] = "";
        char id[256] = "";

        get_wmi_string(obj, L"DeviceID", device_id, sizeof(device_id));
        get_wmi_string(obj, L"Name", name, sizeof(name));
        get_wmi_string(obj, L"Status", status, sizeof(status));

        if (*device_id)
            strncpyz(id, device_id, sizeof(id) - 1);
        else if (*name)
            strncpyz(id, name, sizeof(id) - 1);
        else
            snprintfz(id, sizeof(id), "Fan%zu", ++generated_id);

        struct win_fan *fan = get_or_create_fan(id);
        fan->seen = true;
        update_fan_string(&fan->name, *name ? name : id);
        update_fan_string(&fan->device_id, device_id);
        update_fan_string(&fan->status, status);

        fan->desired_speed_valid = get_wmi_uint64(obj, L"DesiredSpeed", &fan->desired_speed);
        fan->active_cooling_valid = get_wmi_bool(obj, L"ActiveCooling", &fan->active_cooling);
        fan->variable_speed_valid = get_wmi_bool(obj, L"VariableSpeed", &fan->variable_speed);

        uint64_t availability = 0;
        bool availability_valid = get_wmi_uint64(obj, L"Availability", &availability);
        fan->online_valid = availability_valid || *status;
        fan->online = (availability_valid && availability == 3) || strcasecmp(status, "OK") == 0;

        update_fan_charts(fan, update_every);
        collected = true;

        obj->lpVtbl->Release(obj);
    }

    pEnumerator->lpVtbl->Release(pEnumerator);

    struct win_fan **pp = &fans_root;
    while (*pp) {
        struct win_fan *fan = *pp;
        if (!fan->seen) {
            *pp = fan->next;
            free_fan(fan);
        } else {
            pp = &fan->next;
        }
    }

    return collected ? 0 : -1;
}
