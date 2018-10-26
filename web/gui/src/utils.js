NETDATA.encodeURIComponent = function (s) {
    if (typeof(s) === 'string') {
        return encodeURIComponent(s);
    }

    return s;
};

/// A heuristic for detecting slow devices.
let isSlowDeviceResult = undefined;
const isSlowDevice = function () {
    if (!isSlowDeviceResult) {
        return isSlowDeviceResult;
    }

    try {
        let ua = navigator.userAgent.toLowerCase();

        let iOS = /ipad|iphone|ipod/.test(ua) && !window.MSStream;
        let android = /android/.test(ua) && !window.MSStream;
        isSlowDeviceResult = (iOS || android);
    } catch (e) {
        isSlowDeviceResult = false;
    }

    return isSlowDeviceResult;
};

NETDATA.guid = function () {
    function s4() {
        return Math.floor((1 + Math.random()) * 0x10000)
            .toString(16)
            .substring(1);
    }

    return s4() + s4() + '-' + s4() + '-' + s4() + '-' + s4() + '-' + s4() + s4() + s4();
};

NETDATA.zeropad = function (x) {
    if (x > -10 && x < 10) {
        return '0' + x.toString();
    } else {
        return x.toString();
    }
};

NETDATA.seconds4human = function (seconds, options) {
    let default_options = {
        now: 'now',
        space: ' ',
        negative_suffix: 'ago',
        day: 'day',
        days: 'days',
        hour: 'hour',
        hours: 'hours',
        minute: 'min',
        minutes: 'mins',
        second: 'sec',
        seconds: 'secs',
        and: 'and'
    };

    if (typeof options !== 'object') {
        options = default_options;
    } else {
        let x;
        for (const x in default_options) {
            if (typeof options[x] !== 'string') {
                options[x] = default_options[x];
            }
        }
    }

    if (typeof seconds === 'string') {
        seconds = parseInt(seconds, 10);
    }

    if (seconds === 0) {
        return options.now;
    }

    let suffix = '';
    if (seconds < 0) {
        seconds = -seconds;
        if (options.negative_suffix !== '') {
            suffix = options.space + options.negative_suffix;
        }
    }

    let days = Math.floor(seconds / 86400);
    seconds -= (days * 86400);

    let hours = Math.floor(seconds / 3600);
    seconds -= (hours * 3600);

    let minutes = Math.floor(seconds / 60);
    seconds -= (minutes * 60);

    let strings = [];

    if (days > 1) {
        strings.push(days.toString() + options.space + options.days);
    } else if (days === 1) {
        strings.push(days.toString() + options.space + options.day);
    }

    if (hours > 1) {
        strings.push(hours.toString() + options.space + options.hours);
    } else if (hours === 1) {
        strings.push(hours.toString() + options.space + options.hour);
    }

    if (minutes > 1) {
        strings.push(minutes.toString() + options.space + options.minutes);
    } else if (minutes === 1) {
        strings.push(minutes.toString() + options.space + options.minute);
    }

    if (seconds > 1) {
        strings.push(Math.floor(seconds).toString() + options.space + options.seconds);
    } else if (seconds === 1) {
        strings.push(Math.floor(seconds).toString() + options.space + options.second);
    }

    if (strings.length === 1) {
        return strings.pop() + suffix;
    }

    let last = strings.pop();
    return strings.join(", ") + " " + options.and + " " + last + suffix;
};

// ----------------------------------------------------------------------------------------------------------------
// element data attributes

NETDATA.dataAttribute = function (element, attribute, def) {
    let key = 'data-' + attribute.toString();
    if (element.hasAttribute(key)) {
        let data = element.getAttribute(key);

        if (data === 'true') {
            return true;
        }
        if (data === 'false') {
            return false;
        }
        if (data === 'null') {
            return null;
        }

        // Only convert to a number if it doesn't change the string
        if (data === +data + '') {
            return +data;
        }

        if (/^(?:\{[\w\W]*\}|\[[\w\W]*\])$/.test(data)) {
            return JSON.parse(data);
        }

        return data;
    } else {
        return def;
    }
};

NETDATA.dataAttributeBoolean = function (element, attribute, def) {
    let value = NETDATA.dataAttribute(element, attribute, def);

    if (value === true || value === false) // gmosx: Love this :)
    {
        return value;
    }

    if (typeof(value) === 'string') {
        if (value === 'yes' || value === 'on') {
            return true;
        }

        if (value === '' || value === 'no' || value === 'off' || value === 'null') {
            return false;
        }

        return def;
    }

    if (typeof(value) === 'number') {
        return value !== 0;
    }

    return def;
};
