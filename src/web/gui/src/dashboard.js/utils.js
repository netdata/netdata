// *** src/dashboard.js/utils.js

NETDATA.name2id = function (s) {
    return s
        .replace(/ /g, '_')
        .replace(/:/g, '_')
        .replace(/\(/g, '_')
        .replace(/\)/g, '_')
        .replace(/\./g, '_')
        .replace(/\//g, '_');
};

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
    let defaultOptions = {
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
        options = defaultOptions;
    } else {
        for (var x in defaultOptions) {
            if (typeof options[x] !== 'string') {
                options[x] = defaultOptions[x];
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

// ----------------------------------------------------------------------------------------------------------------
// fast numbers formatting

NETDATA.fastNumberFormat = {
    formattersFixed: [],
    formattersZeroBased: [],

    // this is the fastest and the preferred
    getIntlNumberFormat: function (min, max) {
        let key = max;
        if (min === max) {
            if (typeof this.formattersFixed[key] === 'undefined') {
                this.formattersFixed[key] = new Intl.NumberFormat(undefined, {
                    // style: 'decimal',
                    // minimumIntegerDigits: 1,
                    // minimumSignificantDigits: 1,
                    // maximumSignificantDigits: 1,
                    useGrouping: true,
                    minimumFractionDigits: min,
                    maximumFractionDigits: max
                });
            }

            return this.formattersFixed[key];
        } else if (min === 0) {
            if (typeof this.formattersZeroBased[key] === 'undefined') {
                this.formattersZeroBased[key] = new Intl.NumberFormat(undefined, {
                    // style: 'decimal',
                    // minimumIntegerDigits: 1,
                    // minimumSignificantDigits: 1,
                    // maximumSignificantDigits: 1,
                    useGrouping: true,
                    minimumFractionDigits: min,
                    maximumFractionDigits: max
                });
            }

            return this.formattersZeroBased[key];
        } else {
            // this is never used
            // it is added just for completeness
            return new Intl.NumberFormat(undefined, {
                // style: 'decimal',
                // minimumIntegerDigits: 1,
                // minimumSignificantDigits: 1,
                // maximumSignificantDigits: 1,
                useGrouping: true,
                minimumFractionDigits: min,
                maximumFractionDigits: max
            });
        }
    },

    // this respects locale
    getLocaleString: function (min, max) {
        let key = max;
        if (min === max) {
            if (typeof this.formattersFixed[key] === 'undefined') {
                this.formattersFixed[key] = {
                    format: function (value) {
                        return value.toLocaleString(undefined, {
                            // style: 'decimal',
                            // minimumIntegerDigits: 1,
                            // minimumSignificantDigits: 1,
                            // maximumSignificantDigits: 1,
                            useGrouping: true,
                            minimumFractionDigits: min,
                            maximumFractionDigits: max
                        });
                    }
                };
            }

            return this.formattersFixed[key];
        } else if (min === 0) {
            if (typeof this.formattersZeroBased[key] === 'undefined') {
                this.formattersZeroBased[key] = {
                    format: function (value) {
                        return value.toLocaleString(undefined, {
                            // style: 'decimal',
                            // minimumIntegerDigits: 1,
                            // minimumSignificantDigits: 1,
                            // maximumSignificantDigits: 1,
                            useGrouping: true,
                            minimumFractionDigits: min,
                            maximumFractionDigits: max
                        });
                    }
                };
            }

            return this.formattersZeroBased[key];
        } else {
            return {
                format: function (value) {
                    return value.toLocaleString(undefined, {
                        // style: 'decimal',
                        // minimumIntegerDigits: 1,
                        // minimumSignificantDigits: 1,
                        // maximumSignificantDigits: 1,
                        useGrouping: true,
                        minimumFractionDigits: min,
                        maximumFractionDigits: max
                    });
                }
            };
        }
    },

    // the fallback
    getFixed: function (min, max) {
        let key = max;
        if (min === max) {
            if (typeof this.formattersFixed[key] === 'undefined') {
                this.formattersFixed[key] = {
                    format: function (value) {
                        if (value === 0) {
                            return "0";
                        }
                        return value.toFixed(max);
                    }
                };
            }

            return this.formattersFixed[key];
        } else if (min === 0) {
            if (typeof this.formattersZeroBased[key] === 'undefined') {
                this.formattersZeroBased[key] = {
                    format: function (value) {
                        if (value === 0) {
                            return "0";
                        }
                        return value.toFixed(max);
                    }
                };
            }

            return this.formattersZeroBased[key];
        } else {
            return {
                format: function (value) {
                    if (value === 0) {
                        return "0";
                    }
                    return value.toFixed(max);
                }
            };
        }
    },

    testIntlNumberFormat: function () {
        let value = 1.12345;
        let e1 = "1.12", e2 = "1,12";
        let s = "";

        try {
            let x = new Intl.NumberFormat(undefined, {
                useGrouping: true,
                minimumFractionDigits: 2,
                maximumFractionDigits: 2
            });

            s = x.format(value);
        } catch (e) {
            s = "";
        }

        // console.log('NumberFormat: ', s);
        return (s === e1 || s === e2);
    },

    testLocaleString: function () {
        let value = 1.12345;
        let e1 = "1.12", e2 = "1,12";
        let s = "";

        try {
            s = value.toLocaleString(undefined, {
                useGrouping: true,
                minimumFractionDigits: 2,
                maximumFractionDigits: 2
            });
        } catch (e) {
            s = "";
        }

        // console.log('localeString: ', s);
        return (s === e1 || s === e2);
    },

    // on first run we decide which formatter to use
    get: function (min, max) {
        if (this.testIntlNumberFormat()) {
            // console.log('numberformat');
            this.get = this.getIntlNumberFormat;
        } else if (this.testLocaleString()) {
            // console.log('localestring');
            this.get = this.getLocaleString;
        } else {
            // console.log('fixed');
            this.get = this.getFixed;
        }
        return this.get(min, max);
    }
};

// ----------------------------------------------------------------------------------------------------------------
// Detect the netdata server

// http://stackoverflow.com/questions/984510/what-is-my-script-src-url
// http://stackoverflow.com/questions/6941533/get-protocol-domain-and-port-from-url
NETDATA._scriptSource = function () {
    let script = null;

    if (typeof document.currentScript !== 'undefined') {
        script = document.currentScript;
    } else {
        const all_scripts = document.getElementsByTagName('script');
        script = all_scripts[all_scripts.length - 1];
    }

    if (typeof script.getAttribute.length !== 'undefined') {
        script = script.src;
    } else {
        script = script.getAttribute('src', -1);
    }

    return script;
};
