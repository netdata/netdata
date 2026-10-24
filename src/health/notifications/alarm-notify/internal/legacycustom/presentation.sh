# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck shell=bash
# Compute presentation only from supplied facts; missing durations remain unknown.
urlencode "$args_host" >/dev/null; url_host=$REPLY
urlencode "$chart" >/dev/null; url_chart=$REPLY
urlencode "$name" >/dev/null; url_name=$REPLY
urlencode "$value_string" >/dev/null; url_value_string=$REPLY
duration_txt=''; non_clear_duration_txt=''
if [[ -n $duration ]]; then duration4human "$duration" >/dev/null; duration_txt=$REPLY; fi
if [[ -n $non_clear_duration ]]; then duration4human "$non_clear_duration" >/dev/null; non_clear_duration_txt=$REPLY; fi
severity=$status
raised_for=''
if [[ -n $old_status && -n $duration_txt ]]; then raised_for="(was ${old_status,,} for ${duration_txt})"; fi
alarm="${summary//_/ } = ${value_string}"
case $status in
    CRITICAL) image="$images_base_url/images/alert-128-red.png"; color='#ca414b' ;;
    WARNING) image="$images_base_url/images/alert-128-orange.png"; color='#ffc107' ;;
    CLEAR) image="$images_base_url/images/check-mark-2-128-green.png"; color='#77ca6d' ;;
esac
if [[ $status == CLEAR ]]; then
    severity=Recovered
    [[ -n $old_status ]] && severity="Recovered from $old_status"
    if [[ -n $duration_txt && -n $non_clear_duration_txt ]] && ((non_clear_duration > duration)); then
        raised_for="(alarm was raised for $non_clear_duration_txt)"
    fi
    alarm="${summary//_/ }"
    [[ -n $raised_for ]] && alarm+=" $raised_for"
elif [[ $old_status == WARNING && $status == CRITICAL ]] || [[ $old_status == CRITICAL && $status == WARNING ]]; then
    if [[ $status == CRITICAL ]]; then severity="Escalated to $status"; else severity="Demoted to $status"; fi
    if [[ -n $duration_txt && -n $non_clear_duration_txt ]] && ((non_clear_duration > duration)); then
        raised_for="(alarm is raised for $non_clear_duration_txt)"
    fi
else
    raised_for=''
fi
