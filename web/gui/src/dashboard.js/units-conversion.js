NETDATA.unitsConversion = {
    keys: {},       // keys for data-common-units
    latest: {},     // latest selected units for data-common-units

    globalReset: function () {
        this.keys = {};
        this.latest = {};
    },

    scalableUnits: {
        'packets/s': {
            'pps': 1,
            'Kpps': 1000,
            'Mpps': 1000000
        },
        'pps': {
            'pps': 1,
            'Kpps': 1000,
            'Mpps': 1000000
        },
        'kilobits/s': {
            'bits/s': 1 / 1000,
            'kilobits/s': 1,
            'megabits/s': 1000,
            'gigabits/s': 1000000,
            'terabits/s': 1000000000
        },
        'kilobytes/s': {
            'bytes/s': 1 / 1024,
            'kilobytes/s': 1,
            'megabytes/s': 1024,
            'gigabytes/s': 1024 * 1024,
            'terabytes/s': 1024 * 1024 * 1024
        },
        'KB/s': {
            'B/s': 1 / 1024,
            'KB/s': 1,
            'MB/s': 1024,
            'GB/s': 1024 * 1024,
            'TB/s': 1024 * 1024 * 1024
        },
        'KB': {
            'B': 1 / 1024,
            'KB': 1,
            'MB': 1024,
            'GB': 1024 * 1024,
            'TB': 1024 * 1024 * 1024
        },
        'MB': {
            'B': 1 / (1024 * 1024),
            'KB': 1 / 1024,
            'MB': 1,
            'GB': 1024,
            'TB': 1024 * 1024,
            'PB': 1024 * 1024 * 1024
        },
        'GB': {
            'B': 1 / (1024 * 1024 * 1024),
            'KB': 1 / (1024 * 1024),
            'MB': 1 / 1024,
            'GB': 1,
            'TB': 1024,
            'PB': 1024 * 1024,
            'EB': 1024 * 1024 * 1024
        }
        /*
        'milliseconds': {
            'seconds': 1000
        },
        'seconds': {
            'milliseconds': 0.001,
            'seconds': 1,
            'minutes': 60,
            'hours': 3600,
            'days': 86400
        }
        */
    },

    convertibleUnits: {
        'Celsius': {
            'Fahrenheit': {
                check: function (max) {
                    void(max);
                    return NETDATA.options.current.temperature === 'fahrenheit';
                },
                convert: function (value) {
                    return value * 9 / 5 + 32;
                }
            }
        },
        'celsius': {
            'fahrenheit': {
                check: function (max) {
                    void(max);
                    return NETDATA.options.current.temperature === 'fahrenheit';
                },
                convert: function (value) {
                    return value * 9 / 5 + 32;
                }
            }
        },
        'seconds': {
            'time': {
                check: function (max) {
                    void(max);
                    return NETDATA.options.current.seconds_as_time;
                },
                convert: function (seconds) {
                    return NETDATA.unitsConversion.seconds2time(seconds);
                }
            }
        },
        'milliseconds': {
            'milliseconds': {
                check: function (max) {
                    return NETDATA.options.current.seconds_as_time && max < 1000;
                },
                convert: function (milliseconds) {
                    let tms = Math.round(milliseconds * 10);
                    milliseconds = Math.floor(tms / 10);

                    tms -= milliseconds * 10;

                    return (milliseconds).toString() + '.' + tms.toString();
                }
            },
            'seconds': {
                check: function (max) {
                    return NETDATA.options.current.seconds_as_time && max >= 1000 && max < 60000;
                },
                convert: function (milliseconds) {
                    milliseconds = Math.round(milliseconds);

                    let seconds = Math.floor(milliseconds / 1000);
                    milliseconds -= seconds * 1000;

                    milliseconds = Math.round(milliseconds / 10);

                    return seconds.toString() + '.'
                        + NETDATA.zeropad(milliseconds);
                }
            },
            'M:SS.ms': {
                check: function (max) {
                    return NETDATA.options.current.seconds_as_time && max >= 60000;
                },
                convert: function (milliseconds) {
                    milliseconds = Math.round(milliseconds);

                    let minutes = Math.floor(milliseconds / 60000);
                    milliseconds -= minutes * 60000;

                    let seconds = Math.floor(milliseconds / 1000);
                    milliseconds -= seconds * 1000;

                    milliseconds = Math.round(milliseconds / 10);

                    return minutes.toString() + ':'
                        + NETDATA.zeropad(seconds) + '.'
                        + NETDATA.zeropad(milliseconds);
                }
            }
        }
    },

    seconds2time: function (seconds) {
        seconds = Math.abs(seconds);

        let days = Math.floor(seconds / 86400);
        seconds -= days * 86400;

        let hours = Math.floor(seconds / 3600);
        seconds -= hours * 3600;

        let minutes = Math.floor(seconds / 60);
        seconds -= minutes * 60;

        seconds = Math.round(seconds);

        let ms_txt = '';
        /*
        let ms = seconds - Math.floor(seconds);
        seconds -= ms;
        ms = Math.round(ms * 1000);

        if (ms > 1) {
            if (ms < 10)
                ms_txt = '.00' + ms.toString();
            else if (ms < 100)
                ms_txt = '.0' + ms.toString();
            else
                ms_txt = '.' + ms.toString();
        }
        */

        return ((days > 0) ? days.toString() + 'd:' : '').toString()
            + NETDATA.zeropad(hours) + ':'
            + NETDATA.zeropad(minutes) + ':'
            + NETDATA.zeropad(seconds)
            + ms_txt;
    },

    // get a function that converts the units
    // + every time units are switched call the callback
    get: function (uuid, min, max, units, desired_units, common_units_name, switch_units_callback) {
        // validate the parameters
        if (typeof units === 'undefined') {
            units = 'undefined';
        }

        // check if we support units conversion
        if (typeof this.scalableUnits[units] === 'undefined' && typeof this.convertibleUnits[units] === 'undefined') {
            // we can't convert these units
            //console.log('DEBUG: ' + uuid.toString() + ' can\'t convert units: ' + units.toString());
            return function (value) {
                return value;
            };
        }

        // check if the caller wants the original units
        if (typeof desired_units === 'undefined' || desired_units === null || desired_units === 'original' || desired_units === units) {
            //console.log('DEBUG: ' + uuid.toString() + ' original units wanted');
            switch_units_callback(units);
            return function (value) {
                return value;
            };
        }

        // now we know we can convert the units
        // and the caller wants some kind of conversion

        let tunits = null;
        let tdivider = 0;

        if (typeof this.scalableUnits[units] !== 'undefined') {
            // units that can be scaled
            // we decide a divider

            // console.log('NETDATA.unitsConversion.get(' + units.toString() + ', ' + desired_units.toString() + ', function()) decide divider with min = ' + min.toString() + ', max = ' + max.toString());

            if (desired_units === 'auto') {
                // the caller wants to auto-scale the units

                // find the absolute maximum value that is rendered on the chart
                // based on this we decide the scale
                min = Math.abs(min);
                max = Math.abs(max);
                if (min > max) {
                    max = min;
                }

                // find the smallest scale that provides integers
                // for (x in this.scalableUnits[units]) {
                //     if (this.scalableUnits[units].hasOwnProperty(x)) {
                //         let m = this.scalableUnits[units][x];
                //         if (m <= max && m > tdivider) {
                //             tunits = x;
                //             tdivider = m;
                //         }
                //     }
                // }
                const sunit = this.scalableUnits[units];
                for (const x of Object.keys(sunit)) {
                    let m = sunit[x];
                    if (m <= max && m > tdivider) {
                        tunits = x;
                        tdivider = m;
                    }
                }

                if (tunits === null || tdivider <= 0) {
                    // we couldn't find one
                    //console.log('DEBUG: ' + uuid.toString() + ' cannot find an auto-scaling candidate for units: ' + units.toString() + ' (max: ' + max.toString() + ')');
                    switch_units_callback(units);
                    return function (value) {
                        return value;
                    };
                }

                if (typeof common_units_name === 'string' && typeof uuid === 'string') {
                    // the caller wants several charts to have the same units
                    // data-common-units

                    let common_units_key = common_units_name + '-' + units;

                    // add our divider into the list of keys
                    let t = this.keys[common_units_key];
                    if (typeof t === 'undefined') {
                        this.keys[common_units_key] = {};
                        t = this.keys[common_units_key];
                    }
                    t[uuid] = {
                        units: tunits,
                        divider: tdivider
                    };

                    // find the max divider of all charts
                    let common_units = t[uuid];
                    for (const x in t) {
                        if (t.hasOwnProperty(x) && t[x].divider > common_units.divider) {
                            common_units = t[x];
                        }
                    }

                    // save our common_max to the latest keys
                    let latest = this.latest[common_units_key];
                    if (typeof latest === 'undefined') {
                        this.latest[common_units_key] = {};
                        latest = this.latest[common_units_key];
                    }
                    latest.units = common_units.units;
                    latest.divider = common_units.divider;

                    tunits = latest.units;
                    tdivider = latest.divider;

                    //console.log('DEBUG: ' + uuid.toString() + ' converted units: ' + units.toString() + ' to units: ' + tunits.toString() + ' with divider ' + tdivider.toString() + ', common-units=' + common_units_name.toString() + ((t[uuid].divider !== tdivider)?' USED COMMON, mine was ' + t[uuid].units:' set common').toString());

                    // apply it to this chart
                    switch_units_callback(tunits);
                    return function (value) {
                        if (tdivider !== latest.divider) {
                            // another chart switched our common units
                            // we should switch them too
                            //console.log('DEBUG: ' + uuid + ' switching units due to a common-units change, from ' + tunits.toString() + ' to ' + latest.units.toString());
                            tunits = latest.units;
                            tdivider = latest.divider;
                            switch_units_callback(tunits);
                        }

                        return value / tdivider;
                    };
                } else {
                    // the caller did not give data-common-units
                    // this chart auto-scales independently of all others
                    //console.log('DEBUG: ' + uuid.toString() + ' converted units: ' + units.toString() + ' to units: ' + tunits.toString() + ' with divider ' + tdivider.toString() + ', autonomously');

                    switch_units_callback(tunits);
                    return function (value) {
                        return value / tdivider;
                    };
                }
            } else {
                // the caller wants specific units

                if (typeof this.scalableUnits[units][desired_units] !== 'undefined') {
                    // all good, set the new units
                    tdivider = this.scalableUnits[units][desired_units];
                    // console.log('DEBUG: ' + uuid.toString() + ' converted units: ' + units.toString() + ' to units: ' + desired_units.toString() + ' with divider ' + tdivider.toString() + ', by reference');
                    switch_units_callback(desired_units);
                    return function (value) {
                        return value / tdivider;
                    };
                } else {
                    // oops! switch back to original units
                    console.log('Units conversion from ' + units.toString() + ' to ' + desired_units.toString() + ' is not supported.');
                    switch_units_callback(units);
                    return function (value) {
                        return value;
                    };
                }
            }
        } else if (typeof this.convertibleUnits[units] !== 'undefined') {
            // units that can be converted
            if (desired_units === 'auto') {
                for (const x in this.convertibleUnits[units]) {
                    if (this.convertibleUnits[units].hasOwnProperty(x)) {
                        if (this.convertibleUnits[units][x].check(max)) {
                            //console.log('DEBUG: ' + uuid.toString() + ' converting ' + units.toString() + ' to: ' + x.toString());
                            switch_units_callback(x);
                            return this.convertibleUnits[units][x].convert;
                        }
                    }
                }

                // none checked ok
                //console.log('DEBUG: ' + uuid.toString() + ' no conversion available for ' + units.toString() + ' to: ' + desired_units.toString());
                switch_units_callback(units);
                return function (value) {
                    return value;
                };
            } else if (typeof this.convertibleUnits[units][desired_units] !== 'undefined') {
                switch_units_callback(desired_units);
                return this.convertibleUnits[units][desired_units].convert;
            } else {
                console.log('Units conversion from ' + units.toString() + ' to ' + desired_units.toString() + ' is not supported.');
                switch_units_callback(units);
                return function (value) {
                    return value;
                };
            }
        } else {
            // hm... did we forget to implement the new type?
            console.log(`Unmatched unit conversion method for units ${units.toString()}`);
            switch_units_callback(units);
            return function (value) {
                return value;
            };
        }
    }
};
