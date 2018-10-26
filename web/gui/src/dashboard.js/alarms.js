
// Registry of netdata hosts

NETDATA.alarms = {
    onclick: null,                  // the callback to handle the click - it will be called with the alarm log entry
    chart_div_offset: -50,          // give that space above the chart when scrolling to it
    chart_div_id_prefix: 'chart_',  // the chart DIV IDs have this prefix (they should be NETDATA.name2id(chart.id))
    chart_div_animation_duration: 0,// the duration of the animation while scrolling to a chart

    ms_penalty: 0,                  // the time penalty of the next alarm
    ms_between_notifications: 500,  // firefox moves the alarms off-screen (above, outside the top of the screen)
                                    // if alarms are shown faster than: one per 500ms

    update_every: 10000,            // the time in ms between alarm checks

    notifications: false,           // when true, the browser supports notifications (may not be granted though)
    last_notification_id: 0,        // the id of the last alarm_log we have raised an alarm for
    first_notification_id: 0,       // the id of the first alarm_log entry for this session
                                    // this is used to prevent CLEAR notifications for past events
    // notifications_shown: [],

    server: null,                   // the server to connect to for fetching alarms
    current: null,                  // the list of raised alarms - updated in the background

    // a callback function to call every time the list of raised alarms is refreshed
    callback: (typeof netdataAlarmsActiveCallback === 'function') ? netdataAlarmsActiveCallback : null,

    // a callback function to call every time a notification is shown
    // the return value is used to decide if the notification will be shown
    notificationCallback: (typeof netdataAlarmsNotifCallback === 'function') ? netdataAlarmsNotifCallback : null,

    recipients: null,               // the list (array) of recipients to show alarms for, or null

    recipientMatches: function (to_string, wanted_array) {
        if (typeof wanted_array === 'undefined' || wanted_array === null || Array.isArray(wanted_array) === false) {
            return true;
        }

        let r = ' ' + to_string.toString() + ' ';
        let len = wanted_array.length;
        while (len--) {
            if (r.indexOf(' ' + wanted_array[len] + ' ') >= 0) {
                return true;
            }
        }

        return false;
    },

    activeForRecipients: function () {
        let active = {};
        let data = NETDATA.alarms.current;

        if (typeof data === 'undefined' || data === null) {
            return active;
        }

        for (let x in data.alarms) {
            if (!data.alarms.hasOwnProperty(x)) {
                continue;
            }

            let alarm = data.alarms[x];
            if ((alarm.status === 'WARNING' || alarm.status === 'CRITICAL') && NETDATA.alarms.recipientMatches(alarm.recipient, NETDATA.alarms.recipients)) {
                active[x] = alarm;
            }
        }

        return active;
    },

    notify: function (entry) {
        // console.log('alarm ' + entry.unique_id);

        if (entry.updated) {
            // console.log('alarm ' + entry.unique_id + ' has been updated by another alarm');
            return;
        }

        let value_string = entry.value_string;

        if (NETDATA.alarms.current !== null) {
            // get the current value_string
            let t = NETDATA.alarms.current.alarms[entry.chart + '.' + entry.name];
            if (typeof t !== 'undefined' && entry.status === t.status && typeof t.value_string !== 'undefined') {
                value_string = t.value_string;
            }
        }

        let name = entry.name.replace(/_/g, ' ');
        let status = entry.status.toLowerCase();
        let title = name + ' = ' + value_string.toString();
        let tag = entry.alarm_id;
        let icon = 'images/seo-performance-128.png';
        let interaction = false;
        let data = entry;
        let show = true;

        // console.log('alarm ' + entry.unique_id + ' ' + entry.chart + '.' + entry.name + ' is ' +  entry.status);

        switch (entry.status) {
            case 'REMOVED':
                show = false;
                break;

            case 'UNDEFINED':
                return;

            case 'UNINITIALIZED':
                return;

            case 'CLEAR':
                if (entry.unique_id < NETDATA.alarms.first_notification_id) {
                    // console.log('alarm ' + entry.unique_id + ' is not current');
                    return;
                }
                if (entry.old_status === 'UNINITIALIZED' || entry.old_status === 'UNDEFINED') {
                    // console.log('alarm' + entry.unique_id + ' switch to CLEAR from ' + entry.old_status);
                    return;
                }
                if (entry.no_clear_notification) {
                    // console.log('alarm' + entry.unique_id + ' is CLEAR but has no_clear_notification flag');
                    return;
                }
                title = name + ' back to normal (' + value_string.toString() + ')';
                icon = 'images/check-mark-2-128-green.png';
                interaction = false;
                break;

            case 'WARNING':
                if (entry.old_status === 'CRITICAL') {
                    status = 'demoted to ' + entry.status.toLowerCase();
                }

                icon = 'images/alert-128-orange.png';
                interaction = false;
                break;

            case 'CRITICAL':
                if (entry.old_status === 'WARNING') {
                    status = 'escalated to ' + entry.status.toLowerCase();
                }

                icon = 'images/alert-128-red.png';
                interaction = true;
                break;

            default:
                console.log('invalid alarm status ' + entry.status);
                return;
        }

        // filter recipients
        if (show) {
            show = NETDATA.alarms.recipientMatches(entry.recipient, NETDATA.alarms.recipients);
        }

        /*
        // cleanup old notifications with the same alarm_id as this one
        // it does not seem to work on any web browser - so notifications cannot be removed

        let len = NETDATA.alarms.notifications_shown.length;
        while (len--) {
            let n = NETDATA.alarms.notifications_shown[len];
            if (n.data.alarm_id === entry.alarm_id) {
                console.log('removing old alarm ' + n.data.unique_id);

                // close the notification
                n.close.bind(n);

                // remove it from the array
                NETDATA.alarms.notifications_shown.splice(len, 1);
                len = NETDATA.alarms.notifications_shown.length;
            }
        }
        */

        if (show) {
            if (typeof NETDATA.alarms.notificationCallback === 'function') {
                show = NETDATA.alarms.notificationCallback(entry);
            }

            if (show) {
                setTimeout(function () {
                    // show this notification
                    // console.log('new notification: ' + title);
                    let n = new Notification(title, {
                        body: entry.hostname + ' - ' + entry.chart + ' (' + entry.family + ') - ' + status + ': ' + entry.info,
                        tag: tag,
                        requireInteraction: interaction,
                        icon: NETDATA.serverStatic + icon,
                        data: data
                    });

                    n.onclick = function (event) {
                        event.preventDefault();
                        NETDATA.alarms.onclick(event.target.data);
                    };

                    // console.log(n);
                    // NETDATA.alarms.notifications_shown.push(n);
                    // console.log(entry);
                }, NETDATA.alarms.ms_penalty);

                NETDATA.alarms.ms_penalty += NETDATA.alarms.ms_between_notifications;
            }
        }
    },

    scrollToChart: function (chart_id) {
        if (typeof chart_id === 'string') {
            let offset = $('#' + NETDATA.alarms.chart_div_id_prefix + NETDATA.name2id(chart_id)).offset();
            if (typeof offset !== 'undefined') {
                $('html, body').animate({scrollTop: offset.top + NETDATA.alarms.chart_div_offset}, NETDATA.alarms.chart_div_animation_duration);
                return true;
            }
        }
        return false;
    },

    scrollToAlarm: function (alarm) {
        if (typeof alarm === 'object') {
            let ret = NETDATA.alarms.scrollToChart(alarm.chart);

            if (ret && NETDATA.options.page_is_visible === false) {
                window.focus();
            }
            //    alert('netdata dashboard will now scroll to chart: ' + alarm.chart + '\n\nThis alarm opened to bring the browser window in front of the screen. Click on the dashboard to prevent it from appearing again.');
        }

    },

    notifyAll: function () {
        // console.log('FETCHING ALARM LOG');
        NETDATA.alarms.get_log(NETDATA.alarms.last_notification_id, function (data) {
            // console.log('ALARM LOG FETCHED');

            if (data === null || typeof data !== 'object') {
                console.log('invalid alarms log response');
                return;
            }

            if (data.length === 0) {
                console.log('received empty alarm log');
                return;
            }

            // console.log('received alarm log of ' + data.length + ' entries, from ' + data[data.length - 1].unique_id.toString() + ' to ' + data[0].unique_id.toString());

            data.sort(function (a, b) {
                if (a.unique_id > b.unique_id) {
                    return -1;
                }
                if (a.unique_id < b.unique_id) {
                    return 1;
                }
                return 0;
            });

            NETDATA.alarms.ms_penalty = 0;

            let len = data.length;
            while (len--) {
                if (data[len].unique_id > NETDATA.alarms.last_notification_id) {
                    NETDATA.alarms.notify(data[len]);
                }
                //else
                //    console.log('ignoring alarm (older) with id ' + data[len].unique_id.toString());
            }

            NETDATA.alarms.last_notification_id = data[0].unique_id;

            if (typeof netdataAlarmsRemember === 'undefined' || netdataAlarmsRemember) {
                NETDATA.localStorageSet('last_notification_id', NETDATA.alarms.last_notification_id, null);
            }
            // console.log('last notification id = ' + NETDATA.alarms.last_notification_id);
        })
    },

    check_notifications: function () {
        // returns true if we should fire 1+ notifications

        if (NETDATA.alarms.notifications !== true) {
            // console.log('web notifications are not available');
            return false;
        }

        if (Notification.permission !== 'granted') {
            // console.log('web notifications are not granted');
            return false;
        }

        if (typeof NETDATA.alarms.current !== 'undefined' && typeof NETDATA.alarms.current.alarms === 'object') {
            // console.log('can do alarms: old id = ' + NETDATA.alarms.last_notification_id + ' new id = ' + NETDATA.alarms.current.latest_alarm_log_unique_id);

            if (NETDATA.alarms.current.latest_alarm_log_unique_id > NETDATA.alarms.last_notification_id) {
                // console.log('new alarms detected');
                return true;
            }
            //else console.log('no new alarms');
        }
        // else console.log('cannot process alarms');

        return false;
    },

    get: function (what, callback) {
        $.ajax({
            url: NETDATA.alarms.server + '/api/v1/alarms?' + what.toString(),
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkOptional('/api/v1/alarms', data /*, '.*\.(calc|calc_parsed|warn|warn_parsed|crit|crit_parsed)$' */);

                if (NETDATA.alarms.first_notification_id === 0 && typeof data.latest_alarm_log_unique_id === 'number') {
                    NETDATA.alarms.first_notification_id = data.latest_alarm_log_unique_id;
                }

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(415, NETDATA.alarms.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    update_forever: function () {
        if (netdataShowAlarms !== true || netdataSnapshotData !== null) {
            return;
        }

        NETDATA.alarms.get('active', function (data) {
            if (data !== null) {
                NETDATA.alarms.current = data;

                if (NETDATA.alarms.check_notifications()) {
                    NETDATA.alarms.notifyAll();
                }

                if (typeof NETDATA.alarms.callback === 'function') {
                    NETDATA.alarms.callback(data);
                }

                // Health monitoring is disabled on this netdata
                if (data.status === false) {
                    return;
                }
            }

            setTimeout(NETDATA.alarms.update_forever, NETDATA.alarms.update_every);
        });
    },

    get_log: function (last_id, callback) {
        // console.log('fetching all log after ' + last_id.toString());
        $.ajax({
            url: NETDATA.alarms.server + '/api/v1/alarm_log?after=' + last_id.toString(),
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkOptional('/api/v1/alarm_log', data);

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(416, NETDATA.alarms.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    init: function () {
        NETDATA.alarms.server = NETDATA.fixHost(NETDATA.serverDefault);

        if (typeof netdataAlarmsRemember === 'undefined' || netdataAlarmsRemember) {
            NETDATA.alarms.last_notification_id =
                NETDATA.localStorageGet('last_notification_id', NETDATA.alarms.last_notification_id, null);
        }

        if (NETDATA.alarms.onclick === null) {
            NETDATA.alarms.onclick = NETDATA.alarms.scrollToAlarm;
        }

        if (typeof netdataAlarmsRecipients !== 'undefined' && Array.isArray(netdataAlarmsRecipients)) {
            NETDATA.alarms.recipients = netdataAlarmsRecipients;
        }

        if (netdataShowAlarms) {
            NETDATA.alarms.update_forever();

            if ('Notification' in window) {
                // console.log('notifications available');
                NETDATA.alarms.notifications = true;

                if (Notification.permission === 'default') {
                    Notification.requestPermission();
                }
            }
        }
    }
};
