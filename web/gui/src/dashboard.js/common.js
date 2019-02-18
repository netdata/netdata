
// Compute common (joint) values over multiple charts.


// commonMin & commonMax

NETDATA.commonMin = {
    keys: {},
    latest: {},

    globalReset: function () {
        this.keys = {};
        this.latest = {};
    },

    get: function (state) {
        if (typeof state.tmp.__commonMin === 'undefined') {
            // get the commonMin setting
            state.tmp.__commonMin = NETDATA.dataAttribute(state.element, 'common-min', null);
        }

        let min = state.data.min;
        let name = state.tmp.__commonMin;

        if (name === null) {
            // we don't need commonMin
            //state.log('no need for commonMin');
            return min;
        }

        let t = this.keys[name];
        if (typeof t === 'undefined') {
            // add our commonMin
            this.keys[name] = {};
            t = this.keys[name];
        }

        let uuid = state.uuid;
        if (typeof t[uuid] !== 'undefined') {
            if (t[uuid] === min) {
                //state.log('commonMin ' + state.tmp.__commonMin + ' not changed: ' + this.latest[name]);
                return this.latest[name];
            } else if (min < this.latest[name]) {
                //state.log('commonMin ' + state.tmp.__commonMin + ' increased: ' + min);
                t[uuid] = min;
                this.latest[name] = min;
                return min;
            }
        }

        // add our min
        t[uuid] = min;

        // find the common min
        let m = min;
        // for (let i in t) {
        //     if (t.hasOwnProperty(i) && t[i] < m) m = t[i];
        // }
        for (var ti of Object.values(t)) {
            if (ti < m) {
                m = ti;
            }
        }

        //state.log('commonMin ' + state.tmp.__commonMin + ' updated: ' + m);
        this.latest[name] = m;
        return m;
    }
};

NETDATA.commonMax = {
    keys: {},
    latest: {},

    globalReset: function () {
        this.keys = {};
        this.latest = {};
    },

    get: function (state) {
        if (typeof state.tmp.__commonMax === 'undefined') {
            // get the commonMax setting
            state.tmp.__commonMax = NETDATA.dataAttribute(state.element, 'common-max', null);
        }

        let max = state.data.max;
        let name = state.tmp.__commonMax;

        if (name === null) {
            // we don't need commonMax
            //state.log('no need for commonMax');
            return max;
        }

        let t = this.keys[name];
        if (typeof t === 'undefined') {
            // add our commonMax
            this.keys[name] = {};
            t = this.keys[name];
        }

        let uuid = state.uuid;
        if (typeof t[uuid] !== 'undefined') {
            if (t[uuid] === max) {
                //state.log('commonMax ' + state.tmp.__commonMax + ' not changed: ' + this.latest[name]);
                return this.latest[name];
            } else if (max > this.latest[name]) {
                //state.log('commonMax ' + state.tmp.__commonMax + ' increased: ' + max);
                t[uuid] = max;
                this.latest[name] = max;
                return max;
            }
        }

        // add our max
        t[uuid] = max;

        // find the common max
        let m = max;
        // for (let i in t) {
        //     if (t.hasOwnProperty(i) && t[i] > m) m = t[i];
        // }
        for (var ti of Object.values(t)) {
            if (ti > m) {
                m = ti;
            }
        }

        //state.log('commonMax ' + state.tmp.__commonMax + ' updated: ' + m);
        this.latest[name] = m;
        return m;
    }
};

NETDATA.commonColors = {
    keys: {},

    globalReset: function () {
        this.keys = {};
    },

    get: function (state, label) {
        let ret = this.refill(state);

        if (typeof ret.assigned[label] === 'undefined') {
            ret.assigned[label] = ret.available.shift();
        }

        return ret.assigned[label];
    },

    refill: function (state) {
        let ret, len;

        if (typeof state.tmp.__commonColors === 'undefined') {
            ret = this.prepare(state);
        } else {
            ret = this.keys[state.tmp.__commonColors];
            if (typeof ret === 'undefined') {
                ret = this.prepare(state);
            }
        }

        if (ret.available.length === 0) {
            if (ret.copy_theme || ret.custom.length === 0) {
                // copy the theme colors
                len = NETDATA.themes.current.colors.length;
                while (len--) {
                    ret.available.unshift(NETDATA.themes.current.colors[len]);
                }
            }

            // copy the custom colors
            len = ret.custom.length;
            while (len--) {
                ret.available.unshift(ret.custom[len]);
            }
        }

        state.colors_assigned = ret.assigned;
        state.colors_available = ret.available;
        state.colors_custom = ret.custom;

        return ret;
    },

    __read_custom_colors: function (state, ret) {
        // add the user supplied colors
        let c = NETDATA.dataAttribute(state.element, 'colors', undefined);
        if (typeof c === 'string' && c.length > 0) {
            c = c.split(' ');
            let len = c.length;

            if (len > 0 && c[len - 1] === 'ONLY') {
                len--;
                ret.copy_theme = false;
            }

            while (len--) {
                ret.custom.unshift(c[len]);
            }
        }
    },

    prepare: function (state) {
        let has_custom_colors = false;

        if (typeof state.tmp.__commonColors === 'undefined') {
            let defname = state.chart.context;

            // if this chart has data-colors=""
            // we should use the chart uuid as the default key (private palette)
            // (data-common-colors="NAME" will be used anyways)
            let c = NETDATA.dataAttribute(state.element, 'colors', undefined);
            if (typeof c === 'string' && c.length > 0) {
                defname = state.uuid;
                has_custom_colors = true;
            }

            // get the commonColors setting
            state.tmp.__commonColors = NETDATA.dataAttribute(state.element, 'common-colors', defname);
        }

        let name = state.tmp.__commonColors;
        let ret = this.keys[name];

        if (typeof ret === 'undefined') {
            // add our commonMax
            this.keys[name] = {
                assigned: {},       // name-value of dimensions and their colors
                available: [],      // an array of colors available to be used
                custom: [],         // the array of colors defined by the user
                charts: {},         // the charts linked to this
                copy_theme: true
            };
            ret = this.keys[name];
        }

        if (typeof ret.charts[state.uuid] === 'undefined') {
            ret.charts[state.uuid] = state;

            if (has_custom_colors) {
                this.__read_custom_colors(state, ret);
            }
        }

        return ret;
    }
};
