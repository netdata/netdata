// ----------------------------------------------------------------------------------------------------------------
// XSS checks

NETDATA.xss = {
    enabled: (typeof netdataCheckXSS === 'undefined') ? false : netdataCheckXSS,
    enabled_for_data: (typeof netdataCheckXSS === 'undefined') ? false : netdataCheckXSS,

    string: function (s) {
        return s.toString()
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    },

    object: function (name, obj, ignore_regex) {
        if (typeof ignore_regex !== 'undefined' && ignore_regex.test(name)) {
            // console.log('XSS: ignoring "' + name + '"');
            return obj;
        }

        switch (typeof(obj)) {
            case 'string':
                const ret = this.string(obj);
                if (ret !== obj) {
                    console.log('XSS protection changed string ' + name + ' from "' + obj + '" to "' + ret + '"');
                }
                return ret;

            case 'object':
                if (obj === null) {
                    return obj;
                }

                if (Array.isArray(obj)) {
                    // console.log('checking array "' + name + '"');

                    let len = obj.length;
                    while (len--) {
                        obj[len] = this.object(name + '[' + len + ']', obj[len], ignore_regex);
                    }
                } else {
                    // console.log('checking object "' + name + '"');

                    for (var i in obj) {
                        if (obj.hasOwnProperty(i) === false) {
                            continue;
                        }
                        if (this.string(i) !== i) {
                            console.log('XSS protection removed invalid object member "' + name + '.' + i + '"');
                            delete obj[i];
                        } else {
                            obj[i] = this.object(name + '.' + i, obj[i], ignore_regex);
                        }
                    }
                }
                return obj;

            default:
                return obj;
        }
    },

    checkOptional: function (name, obj, ignore_regex) {
        if (this.enabled) {
            //console.log('XSS: checking optional "' + name + '"...');
            return this.object(name, obj, ignore_regex);
        }
        return obj;
    },

    checkAlways: function (name, obj, ignore_regex) {
        //console.log('XSS: checking always "' + name + '"...');
        return this.object(name, obj, ignore_regex);
    },

    checkData: function (name, obj, ignore_regex) {
        if (this.enabled_for_data) {
            //console.log('XSS: checking data "' + name + '"...');
            return this.object(name, obj, ignore_regex);
        }
        return obj;
    }
};
