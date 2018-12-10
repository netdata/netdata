
// local storage options

NETDATA.localStorage = {
    default: {},
    current: {},
    callback: {} // only used for resetting back to defaults
};

NETDATA.localStorageTested = -1;
NETDATA.localStorageTest = function () {
    if (NETDATA.localStorageTested !== -1) {
        return NETDATA.localStorageTested;
    }

    if (typeof Storage !== "undefined" && typeof localStorage === 'object') {
        let test = 'test';
        try {
            localStorage.setItem(test, test);
            localStorage.removeItem(test);
            NETDATA.localStorageTested = true;
        } catch (e) {
            NETDATA.localStorageTested = false;
        }
    } else {
        NETDATA.localStorageTested = false;
    }

    return NETDATA.localStorageTested;
};

NETDATA.localStorageGet = function (key, def, callback) {
    let ret = def;

    if (typeof NETDATA.localStorage.default[key.toString()] === 'undefined') {
        NETDATA.localStorage.default[key.toString()] = def;
        NETDATA.localStorage.callback[key.toString()] = callback;
    }

    if (NETDATA.localStorageTest()) {
        try {
            // console.log('localStorage: loading "' + key.toString() + '"');
            ret = localStorage.getItem(key.toString());
            // console.log('netdata loaded: ' + key.toString() + ' = ' + ret.toString());
            if (ret === null || ret === 'undefined') {
                // console.log('localStorage: cannot load it, saving "' + key.toString() + '" with value "' + JSON.stringify(def) + '"');
                localStorage.setItem(key.toString(), JSON.stringify(def));
                ret = def;
            } else {
                // console.log('localStorage: got "' + key.toString() + '" with value "' + ret + '"');
                ret = JSON.parse(ret);
                // console.log('localStorage: loaded "' + key.toString() + '" as value ' + ret + ' of type ' + typeof(ret));
            }
        } catch (error) {
            console.log('localStorage: failed to read "' + key.toString() + '", using default: "' + def.toString() + '"');
            ret = def;
        }
    }

    if (typeof ret === 'undefined' || ret === 'undefined') {
        console.log('localStorage: LOADED UNDEFINED "' + key.toString() + '" as value ' + ret + ' of type ' + typeof(ret));
        ret = def;
    }

    NETDATA.localStorage.current[key.toString()] = ret;
    return ret;
};

NETDATA.localStorageSet = function (key, value, callback) {
    if (typeof value === 'undefined' || value === 'undefined') {
        console.log('localStorage: ATTEMPT TO SET UNDEFINED "' + key.toString() + '" as value ' + value + ' of type ' + typeof(value));
    }

    if (typeof NETDATA.localStorage.default[key.toString()] === 'undefined') {
        NETDATA.localStorage.default[key.toString()] = value;
        NETDATA.localStorage.current[key.toString()] = value;
        NETDATA.localStorage.callback[key.toString()] = callback;
    }

    if (NETDATA.localStorageTest()) {
        // console.log('localStorage: saving "' + key.toString() + '" with value "' + JSON.stringify(value) + '"');
        try {
            localStorage.setItem(key.toString(), JSON.stringify(value));
        } catch (e) {
            console.log('localStorage: failed to save "' + key.toString() + '" with value: "' + value.toString() + '"');
        }
    }

    NETDATA.localStorage.current[key.toString()] = value;
    return value;
};

NETDATA.localStorageGetRecursive = function (obj, prefix, callback) {
    let keys = Object.keys(obj);
    let len = keys.length;
    while (len--) {
        let i = keys[len];

        if (typeof obj[i] === 'object') {
            //console.log('object ' + prefix + '.' + i.toString());
            NETDATA.localStorageGetRecursive(obj[i], prefix + '.' + i.toString(), callback);
            continue;
        }

        obj[i] = NETDATA.localStorageGet(prefix + '.' + i.toString(), obj[i], callback);
    }
};

NETDATA.setOption = function (key, value) {
    if (key.toString() === 'setOptionCallback') {
        if (typeof NETDATA.options.current.setOptionCallback === 'function') {
            NETDATA.options.current[key.toString()] = value;
            NETDATA.options.current.setOptionCallback();
        }
    } else if (NETDATA.options.current[key.toString()] !== value) {
        let name = 'options.' + key.toString();

        if (typeof NETDATA.localStorage.default[name.toString()] === 'undefined') {
            console.log('localStorage: setOption() on unsaved option: "' + name.toString() + '", value: ' + value);
        }

        //console.log(NETDATA.localStorage);
        //console.log('setOption: setting "' + key.toString() + '" to "' + value + '" of type ' + typeof(value) + ' original type ' + typeof(NETDATA.options.current[key.toString()]));
        //console.log(NETDATA.options);
        NETDATA.options.current[key.toString()] = NETDATA.localStorageSet(name.toString(), value, null);

        if (typeof NETDATA.options.current.setOptionCallback === 'function') {
            NETDATA.options.current.setOptionCallback();
        }
    }

    return true;
};

NETDATA.getOption = function (key) {
    return NETDATA.options.current[key.toString()];
};

// read settings from local storage
NETDATA.localStorageGetRecursive(NETDATA.options.current, 'options', null);

// always start with this option enabled.
NETDATA.setOption('stop_updates_when_focus_is_lost', true);

NETDATA.resetOptions = function () {
    let keys = Object.keys(NETDATA.localStorage.default);
    let len = keys.length;

    while (len--) {
        let i = keys[len];
        let a = i.split('.');

        if (a[0] === 'options') {
            if (a[1] === 'setOptionCallback') {
                continue;
            }
            if (typeof NETDATA.localStorage.default[i] === 'undefined') {
                continue;
            }
            if (NETDATA.options.current[i] === NETDATA.localStorage.default[i]) {
                continue;
            }

            NETDATA.setOption(a[1], NETDATA.localStorage.default[i]);
        } else if (a[0] === 'chart_heights') {
            if (typeof NETDATA.localStorage.callback[i] === 'function' && typeof NETDATA.localStorage.default[i] !== 'undefined') {
                NETDATA.localStorage.callback[i](NETDATA.localStorage.default[i]);
            }
        }
    }

    NETDATA.dateTime.init(NETDATA.options.current.timezone);
};
