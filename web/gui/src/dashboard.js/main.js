
// *** src/dashboard.js/main.js

if (NETDATA.options.debug.main_loop) {
    console.log('welcome to NETDATA');
}

NETDATA.onresizeCallback = null;
NETDATA.onresize = function () {
    NETDATA.options.last_page_resize = Date.now();
    NETDATA.onscroll();

    if (typeof NETDATA.onresizeCallback === 'function') {
        NETDATA.onresizeCallback();
    }
};

NETDATA.abortAllRefreshes = function () {
    let targets = NETDATA.options.targets;
    let len = targets.length;

    while (len--) {
        if (targets[len].fetching_data) {
            if (typeof targets[len].xhr !== 'undefined') {
                targets[len].xhr.abort();
                targets[len].running = false;
                targets[len].fetching_data = false;
            }
        }
    }
};

NETDATA.onscrollStartDelay = function () {
    NETDATA.options.last_page_scroll = Date.now();

    NETDATA.options.on_scroll_refresher_stop_until =
        NETDATA.options.last_page_scroll
        + (NETDATA.options.current.async_on_scroll ? 1000 : 0);
};

NETDATA.onscrollEndDelay = function () {
    NETDATA.options.on_scroll_refresher_stop_until =
        Date.now()
        + (NETDATA.options.current.async_on_scroll ? NETDATA.options.current.onscroll_worker_duration_threshold : 0);
};

NETDATA.onscroll_updater_timeout_id = undefined;
NETDATA.onscrollUpdater = function () {
    NETDATA.globalSelectionSync.stop();

    if (NETDATA.options.abort_ajax_on_scroll) {
        NETDATA.abortAllRefreshes();
    }

    // when the user scrolls he sees that we have
    // hidden all the not-visible charts
    // using this little function we try to switch
    // the charts back to visible quickly

    if (!NETDATA.intersectionObserver.enabled()) {
        if (!NETDATA.options.current.parallel_refresher) {
            let targets = NETDATA.options.targets;
            let len = targets.length;

            while (len--) {
                if (!targets[len].running) {
                    targets[len].isVisible();
                }
            }
        }
    }

    NETDATA.onscrollEndDelay();
};

NETDATA.scrollUp = false;
NETDATA.scrollY = window.scrollY;
NETDATA.onscroll = function () {
    //console.log('onscroll() begin');

    NETDATA.onscrollStartDelay();
    NETDATA.chartRefresherReschedule();

    NETDATA.scrollUp = (window.scrollY > NETDATA.scrollY);
    NETDATA.scrollY = window.scrollY;

    if (NETDATA.onscroll_updater_timeout_id) {
        NETDATA.timeout.clear(NETDATA.onscroll_updater_timeout_id);
    }

    NETDATA.onscroll_updater_timeout_id = NETDATA.timeout.set(NETDATA.onscrollUpdater, 0);
    //console.log('onscroll() end');
};

NETDATA.supportsPassiveEvents = function () {
    if (NETDATA.options.passive_events === null) {
        let supportsPassive = false;
        try {
            let opts = Object.defineProperty({}, 'passive', {
                get: function () {
                    supportsPassive = true;
                }
            });
            window.addEventListener("test", null, opts);
        } catch (e) {
            console.log('browser does not support passive events');
        }

        NETDATA.options.passive_events = supportsPassive;
    }

    // console.log('passive ' + NETDATA.options.passive_events);
    return NETDATA.options.passive_events;
};

window.addEventListener('resize', NETDATA.onresize, NETDATA.supportsPassiveEvents() ? {passive: true} : false);
window.addEventListener('scroll', NETDATA.onscroll, NETDATA.supportsPassiveEvents() ? {passive: true} : false);
// window.onresize = NETDATA.onresize;
// window.onscroll = NETDATA.onscroll;

// ----------------------------------------------------------------------------------------------------------------
// Global Pan and Zoom on charts

// Using this structure are synchronize all the charts, so that
// when you pan or zoom one, all others are automatically refreshed
// to the same timespan.

NETDATA.globalPanAndZoom = {
    seq: 0,                 // timestamp ms
                            // every time a chart is panned or zoomed
                            // we set the timestamp here
                            // then we use it as a sequence number
                            // to find if other charts are synchronized
                            // to this time-range

    master: null,           // the master chart (state), to which all others
                            // are synchronized

    force_before_ms: null,  // the timespan to sync all other charts
    force_after_ms: null,

    callback: null,

    globalReset: function () {
        this.clearMaster();
        this.seq = 0;
        this.master = null;
        this.force_after_ms = null;
        this.force_before_ms = null;
        this.callback = null;
    },

    delay: function () {
        if (NETDATA.options.debug.globalPanAndZoom) {
            console.log('globalPanAndZoom.delay()');
        }

        NETDATA.options.auto_refresher_stop_until = Date.now() + NETDATA.options.current.global_pan_sync_time;
    },

    // set a new master
    setMaster: function (state, after, before) {
        this.delay();

        if (!NETDATA.options.current.sync_pan_and_zoom) {
            return;
        }

        if (this.master === null) {
            if (NETDATA.options.debug.globalPanAndZoom) {
                console.log('globalPanAndZoom.setMaster(' + state.id + ', ' + after + ', ' + before + ') SET MASTER');
            }
        } else if (this.master !== state) {
            if (NETDATA.options.debug.globalPanAndZoom) {
                console.log('globalPanAndZoom.setMaster(' + state.id + ', ' + after + ', ' + before + ') CHANGED MASTER');
            }

            this.master.resetChart(true, true);
        }

        let now = Date.now();
        this.master = state;
        this.seq = now;
        this.force_after_ms = after;
        this.force_before_ms = before;

        if (typeof this.callback === 'function') {
            this.callback(true, after, before);
        }
    },

    // clear the master
    clearMaster: function () {
        // if (NETDATA.options.debug.globalPanAndZoom === true)
        //     console.log('globalPanAndZoom.clearMaster()');
        if (NETDATA.options.debug.globalPanAndZoom) {
            console.log('globalPanAndZoom.clearMaster()');
        }

        if (this.master !== null) {
            let st = this.master;
            this.master = null;
            st.resetChart();
        }

        this.master = null;
        this.seq = 0;
        this.force_after_ms = null;
        this.force_before_ms = null;
        NETDATA.options.auto_refresher_stop_until = 0;

        if (typeof this.callback === 'function') {
            this.callback(false, 0, 0);
        }
    },

    // is the given state the master of the global
    // pan and zoom sync?
    isMaster: function (state) {
        return (this.master === state);
    },

    // are we currently have a global pan and zoom sync?
    isActive: function () {
        return (this.master !== null && this.force_before_ms !== null && this.force_after_ms !== null && this.seq !== 0);
    },

    // check if a chart, other than the master
    // needs to be refreshed, due to the global pan and zoom
    shouldBeAutoRefreshed: function (state) {
        if (this.master === null || this.seq === 0) {
            return false;
        }

        //if (state.needsRecreation())
        //  return true;

        return (state.tm.pan_and_zoom_seq !== this.seq);
    }
};

// ----------------------------------------------------------------------------------------------------------------
// global chart underlay (time-frame highlighting)

NETDATA.globalChartUnderlay = {
    callback: null,         // what to call when a highlighted range is setup
    after: null,            // highlight after this time
    before: null,           // highlight before this time
    view_after: null,       // the charts after_ms viewport when the highlight was setup
    view_before: null,      // the charts before_ms viewport, when the highlight was setup
    state: null,            // the chart the highlight was setup

    isActive: function () {
        return (this.after !== null && this.before !== null);
    },

    hasViewport: function () {
        return (this.state !== null && this.view_after !== null && this.view_before !== null);
    },

    init: function (state, after, before, view_after, view_before) {
        this.state = (typeof state !== 'undefined') ? state : null;
        this.after = (typeof after !== 'undefined' && after !== null && after > 0) ? after : null;
        this.before = (typeof before !== 'undefined' && before !== null && before > 0) ? before : null;
        this.view_after = (typeof view_after !== 'undefined' && view_after !== null && view_after > 0) ? view_after : null;
        this.view_before = (typeof view_before !== 'undefined' && view_before !== null && view_before > 0) ? view_before : null;
    },

    setup: function () {
        if (this.isActive()) {
            if (this.state === null) {
                this.state = NETDATA.options.targets[0];
            }

            if (typeof this.callback === 'function') {
                this.callback(true, this.after, this.before);
            }
        } else {
            if (typeof this.callback === 'function') {
                this.callback(false, 0, 0);
            }
        }
    },

    set: function (state, after, before, view_after, view_before) {
        if (after > before) {
            let t = after;
            after = before;
            before = t;
        }

        this.init(state, after, before, view_after, view_before);

        // if (this.hasViewport() === true)
        //     NETDATA.globalPanAndZoom.setMaster(this.state, this.view_after, this.view_before);
        if (this.hasViewport()) {
            NETDATA.globalPanAndZoom.setMaster(this.state, this.view_after, this.view_before);
        }

        this.setup();
    },

    clear: function () {
        this.after = null;
        this.before = null;
        this.state = null;
        this.view_after = null;
        this.view_before = null;

        if (typeof this.callback === 'function') {
            this.callback(false, 0, 0);
        }
    },

    focus: function () {
        if (this.isActive() && this.hasViewport()) {
            if (this.state === null) {
                this.state = NETDATA.options.targets[0];
            }

            if (NETDATA.globalPanAndZoom.isMaster(this.state)) {
                NETDATA.globalPanAndZoom.clearMaster();
            }

            NETDATA.globalPanAndZoom.setMaster(this.state, this.view_after, this.view_before, true);
        }
    }
};

// ----------------------------------------------------------------------------------------------------------------
// dimensions selection

// TODO
// move color assignment to dimensions, here

let dimensionStatus = function (parent, label, name_div, value_div, color) {
    this.enabled = false;
    this.parent = parent;
    this.label = label;
    this.name_div = null;
    this.value_div = null;
    this.color = NETDATA.themes.current.foreground;
    this.selected = (parent.unselected_count === 0);

    this.setOptions(name_div, value_div, color);
};

dimensionStatus.prototype.invalidate = function () {
    this.name_div = null;
    this.value_div = null;
    this.enabled = false;
};

dimensionStatus.prototype.setOptions = function (name_div, value_div, color) {
    this.color = color;

    if (this.name_div !== name_div) {
        this.name_div = name_div;
        this.name_div.title = this.label;
        this.name_div.style.setProperty('color', this.color, 'important');
        if (!this.selected) {
            this.name_div.className = 'netdata-legend-name not-selected';
        } else {
            this.name_div.className = 'netdata-legend-name selected';
        }
    }

    if (this.value_div !== value_div) {
        this.value_div = value_div;
        this.value_div.title = this.label;
        this.value_div.style.setProperty('color', this.color, 'important');
        if (!this.selected) {
            this.value_div.className = 'netdata-legend-value not-selected';
        } else {
            this.value_div.className = 'netdata-legend-value selected';
        }
    }

    this.enabled = true;
    this.setHandler();
};

dimensionStatus.prototype.setHandler = function () {
    if (!this.enabled) {
        return;
    }

    let ds = this;

    // this.name_div.onmousedown = this.value_div.onmousedown = function(e) {
    this.name_div.onclick = this.value_div.onclick = function (e) {
        e.preventDefault();
        if (ds.isSelected()) {
            // this is selected
            if (e.shiftKey || e.ctrlKey) {
                // control or shift key is pressed -> unselect this (except is none will remain selected, in which case select all)
                ds.unselect();

                if (ds.parent.countSelected() === 0) {
                    ds.parent.selectAll();
                }
            } else {
                // no key is pressed -> select only this (except if it is the only selected already, in which case select all)
                if (ds.parent.countSelected() === 1) {
                    ds.parent.selectAll();
                } else {
                    ds.parent.selectNone();
                    ds.select();
                }
            }
        }
        else {
            // this is not selected
            if (e.shiftKey || e.ctrlKey) {
                // control or shift key is pressed -> select this too
                ds.select();
            } else {
                // no key is pressed -> select only this
                ds.parent.selectNone();
                ds.select();
            }
        }

        ds.parent.state.redrawChart();
    }
};

dimensionStatus.prototype.select = function () {
    if (!this.enabled) {
        return;
    }

    this.name_div.className = 'netdata-legend-name selected';
    this.value_div.className = 'netdata-legend-value selected';
    this.selected = true;
};

dimensionStatus.prototype.unselect = function () {
    if (!this.enabled) {
        return;
    }

    this.name_div.className = 'netdata-legend-name not-selected';
    this.value_div.className = 'netdata-legend-value hidden';
    this.selected = false;
};

dimensionStatus.prototype.isSelected = function () {
    // return(this.enabled === true && this.selected === true);
    return this.enabled && this.selected;
};

// ----------------------------------------------------------------------------------------------------------------

let dimensionsVisibility = function (state) {
    this.state = state;
    this.len = 0;
    this.dimensions = {};
    this.selected_count = 0;
    this.unselected_count = 0;
};

dimensionsVisibility.prototype.dimensionAdd = function (label, name_div, value_div, color) {
    if (typeof this.dimensions[label] === 'undefined') {
        this.len++;
        this.dimensions[label] = new dimensionStatus(this, label, name_div, value_div, color);
    } else {
        this.dimensions[label].setOptions(name_div, value_div, color);
    }

    return this.dimensions[label];
};

dimensionsVisibility.prototype.dimensionGet = function (label) {
    return this.dimensions[label];
};

dimensionsVisibility.prototype.invalidateAll = function () {
    let keys = Object.keys(this.dimensions);
    let len = keys.length;
    while (len--) {
        this.dimensions[keys[len]].invalidate();
    }
};

dimensionsVisibility.prototype.selectAll = function () {
    let keys = Object.keys(this.dimensions);
    let len = keys.length;
    while (len--) {
        this.dimensions[keys[len]].select();
    }
};

dimensionsVisibility.prototype.countSelected = function () {
    let selected = 0;
    let keys = Object.keys(this.dimensions);
    let len = keys.length;
    while (len--) {
        if (this.dimensions[keys[len]].isSelected()) {
            selected++;
        }
    }

    return selected;
};

dimensionsVisibility.prototype.selectNone = function () {
    let keys = Object.keys(this.dimensions);
    let len = keys.length;
    while (len--) {
        this.dimensions[keys[len]].unselect();
    }
};

dimensionsVisibility.prototype.selected2BooleanArray = function (array) {
    let ret = [];
    this.selected_count = 0;
    this.unselected_count = 0;

    let len = array.length;
    while (len--) {
        let ds = this.dimensions[array[len]];
        if (typeof ds === 'undefined') {
            // console.log(array[i] + ' is not found');
            ret.unshift(false);
        } else if (ds.isSelected()) {
            ret.unshift(true);
            this.selected_count++;
        } else {
            ret.unshift(false);
            this.unselected_count++;
        }
    }

    if (this.selected_count === 0 && this.unselected_count !== 0) {
        this.selectAll();
        return this.selected2BooleanArray(array);
    }

    return ret;
};

// ----------------------------------------------------------------------------------------------------------------
// date/time conversion

NETDATA.dateTime = {
    using_timezone: false,

    // these are the old netdata functions
    // we fallback to these, if the new ones fail

    localeDateStringNative: function (d) {
        return d.toLocaleDateString();
    },

    localeTimeStringNative: function (d) {
        return d.toLocaleTimeString();
    },

    xAxisTimeStringNative: function (d) {
        return NETDATA.zeropad(d.getHours()) + ":"
            + NETDATA.zeropad(d.getMinutes()) + ":"
            + NETDATA.zeropad(d.getSeconds());
    },

    // initialize the new date/time conversion
    // functions.
    // if this fails, we fallback to the above
    init: function (timezone) {
        //console.log('init with timezone: ' + timezone);

        // detect browser timezone
        try {
            NETDATA.options.browser_timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
        } catch (e) {
            console.log('failed to detect browser timezone: ' + e.toString());
            NETDATA.options.browser_timezone = 'cannot-detect-it';
        }

        let ret = false;

        try {
            let dateOptions = {
                localeMatcher: 'best fit',
                formatMatcher: 'best fit',
                weekday: 'short',
                year: 'numeric',
                month: 'short',
                day: '2-digit'
            };

            let timeOptions = {
                localeMatcher: 'best fit',
                hour12: false,
                formatMatcher: 'best fit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            };

            let xAxisOptions = {
                localeMatcher: 'best fit',
                hour12: false,
                formatMatcher: 'best fit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            };

            if (typeof timezone === 'string' && timezone !== '' && timezone !== 'default') {
                dateOptions.timeZone = timezone;
                timeOptions.timeZone = timezone;
                timeOptions.timeZoneName = 'short';
                xAxisOptions.timeZone = timezone;
                this.using_timezone = true;
            } else {
                timezone = 'default';
                this.using_timezone = false;
            }

            this.dateFormat = new Intl.DateTimeFormat(navigator.language, dateOptions);
            this.timeFormat = new Intl.DateTimeFormat(navigator.language, timeOptions);
            this.xAxisFormat = new Intl.DateTimeFormat(navigator.language, xAxisOptions);

            this.localeDateString = function (d) {
                return this.dateFormat.format(d);
            };

            this.localeTimeString = function (d) {
                return this.timeFormat.format(d);
            };

            this.xAxisTimeString = function (d) {
                return this.xAxisFormat.format(d);
            };

            //let d = new Date();
            //let t = this.dateFormat.format(d) + ' ' + this.timeFormat.format(d) + ' ' + this.xAxisFormat.format(d);

            ret = true;
        } catch (e) {
            console.log('Cannot setup Date/Time formatting: ' + e.toString());

            timezone = 'default';
            this.localeDateString = this.localeDateStringNative;
            this.localeTimeString = this.localeTimeStringNative;
            this.xAxisTimeString = this.xAxisTimeStringNative;
            this.using_timezone = false;

            ret = false;
        }

        // save it
        //console.log('init setOption timezone: ' + timezone);
        NETDATA.setOption('timezone', timezone);

        return ret;
    }
};
NETDATA.dateTime.init(NETDATA.options.current.timezone);

// ----------------------------------------------------------------------------------------------------------------
// global selection sync

NETDATA.globalSelectionSync = {
    state: null,
    dontSyncBefore: 0,
    last_t: 0,
    slaves: [],
    timeoutId: undefined,

    globalReset: function () {
        this.stop();
        this.state = null;
        this.dontSyncBefore = 0;
        this.last_t = 0;
        this.slaves = [];
        this.timeoutId = undefined;
    },

    active: function () {
        return (this.state !== null);
    },

    // return true if global selection sync can be enabled now
    enabled: function () {
        // console.log('enabled()');
        // can we globally apply selection sync?
        if (!NETDATA.options.current.sync_selection) {
            return false;
        }

        return (this.dontSyncBefore <= Date.now());
    },

    // set the global selection sync master
    setMaster: function (state) {
        if (!this.enabled()) {
            this.stop();
            return;
        }

        if (this.state === state) {
            return;
        }

        if (this.state !== null) {
            this.stop();
        }

        if (NETDATA.options.debug.globalSelectionSync) {
            console.log('globalSelectionSync.setMaster(' + state.id + ')');
        }

        state.selected = true;
        this.state = state;
        this.last_t = 0;

        // find all slaves
        let targets = NETDATA.intersectionObserver.targets();
        this.slaves = [];
        let len = targets.length;
        while (len--) {
            let st = targets[len];
            if (this.state !== st && st.globalSelectionSyncIsEligible()) {
                this.slaves.push(st);
            }
        }

        // this.delay(100);
    },

    // stop global selection sync
    stop: function () {
        if (this.state !== null) {
            if (NETDATA.options.debug.globalSelectionSync) {
                console.log('globalSelectionSync.stop()');
            }

            let len = this.slaves.length;
            while (len--) {
                this.slaves[len].clearSelection();
            }

            this.state.clearSelection();

            this.last_t = 0;
            this.slaves = [];
            this.state = null;
        }
    },

    // delay global selection sync for some time
    delay: function (ms) {
        if (NETDATA.options.current.sync_selection) {
            // if (NETDATA.options.debug.globalSelectionSync === true) {
            if (NETDATA.options.debug.globalSelectionSync) {
                console.log('globalSelectionSync.delay()');
            }

            if (typeof ms === 'number') {
                this.dontSyncBefore = Date.now() + ms;
            } else {
                this.dontSyncBefore = Date.now() + NETDATA.options.current.sync_selection_delay;
            }
        }
    },

    __syncSlaves: function () {
        // if (NETDATA.globalSelectionSync.enabled() === true) {
        if (NETDATA.globalSelectionSync.enabled()) {
            // if (NETDATA.options.debug.globalSelectionSync === true)
            if (NETDATA.options.debug.globalSelectionSync) {
                console.log('globalSelectionSync.__syncSlaves()');
            }

            let t = NETDATA.globalSelectionSync.last_t;
            let len = NETDATA.globalSelectionSync.slaves.length;
            while (len--) {
                NETDATA.globalSelectionSync.slaves[len].setSelection(t);
            }

            this.timeoutId = undefined;
        }
    },

    // sync all the visible charts to the given time
    // this is to be called from the chart libraries
    sync: function (state, t) {
        // if (NETDATA.options.current.sync_selection === true) {
        if (NETDATA.options.current.sync_selection) {
            // if (NETDATA.options.debug.globalSelectionSync === true)
            if (NETDATA.options.debug.globalSelectionSync) {
                console.log('globalSelectionSync.sync(' + state.id + ', ' + t.toString() + ')');
            }

            this.setMaster(state);

            if (t === this.last_t) {
                return;
            }

            this.last_t = t;

            if (state.foreignElementSelection !== null) {
                state.foreignElementSelection.innerText = NETDATA.dateTime.localeDateString(t) + ' ' + NETDATA.dateTime.localeTimeString(t);
            }

            if (this.timeoutId) {
                NETDATA.timeout.clear(this.timeoutId);
            }

            this.timeoutId = NETDATA.timeout.set(this.__syncSlaves, 0);
        }
    }
};

NETDATA.intersectionObserver = {
    observer: null,
    visible_targets: [],

    options: {
        root: null,
        rootMargin: "0px",
        threshold: null
    },

    enabled: function () {
        return this.observer !== null;
    },

    globalReset: function () {
        if (this.observer !== null) {
            this.visible_targets = [];
            this.observer.disconnect();
            this.init();
        }
    },

    targets: function () {
        if (this.enabled() && this.visible_targets.length > 0) {
            return this.visible_targets;
        } else {
            return NETDATA.options.targets;
        }
    },

    switchChartVisibility: function () {
        let old = this.__visibilityRatioOld;

        if (old !== this.__visibilityRatio) {
            if (old === 0 && this.__visibilityRatio > 0) {
                this.unhideChart();
            } else if (old > 0 && this.__visibilityRatio === 0) {
                this.hideChart();
            }

            this.__visibilityRatioOld = this.__visibilityRatio;
        }
    },

    handler: function (entries, observer) {
        entries.forEach(function (entry) {
            let state = NETDATA.chartState(entry.target);

            let idx;
            if (entry.intersectionRatio > 0) {
                idx = NETDATA.intersectionObserver.visible_targets.indexOf(state);
                if (idx === -1) {
                    if (NETDATA.scrollUp) {
                        NETDATA.intersectionObserver.visible_targets.push(state);
                    } else {
                        NETDATA.intersectionObserver.visible_targets.unshift(state);
                    }
                }
                else if (state.__visibilityRatio === 0) {
                    state.log("was not visible until now, but was already in visible_targets");
                }
            } else {
                idx = NETDATA.intersectionObserver.visible_targets.indexOf(state);
                if (idx !== -1) {
                    NETDATA.intersectionObserver.visible_targets.splice(idx, 1);
                } else if (state.__visibilityRatio > 0) {
                    state.log("was visible, but not found in visible_targets");
                }
            }

            state.__visibilityRatio = entry.intersectionRatio;

            if (!NETDATA.options.current.async_on_scroll) {
                if (window.requestIdleCallback) {
                    window.requestIdleCallback(function () {
                        NETDATA.intersectionObserver.switchChartVisibility.call(state);
                    }, {timeout: 100});
                } else {
                    NETDATA.intersectionObserver.switchChartVisibility.call(state);
                }
            }
        });
    },

    observe: function (state) {
        if (this.enabled()) {
            state.__visibilityRatioOld = 0;
            state.__visibilityRatio = 0;
            this.observer.observe(state.element);

            state.isVisible = function () {
                if (!NETDATA.options.current.update_only_visible) {
                    return true;
                }

                NETDATA.intersectionObserver.switchChartVisibility.call(this);

                return this.__visibilityRatio > 0;
            }
        }
    },

    init: function () {
        if (typeof netdataIntersectionObserver === 'undefined' || netdataIntersectionObserver) {
            try {
                this.observer = new IntersectionObserver(this.handler, this.options);
            } catch (e) {
                console.log("IntersectionObserver is not supported on this browser");
                this.observer = null;
            }
        }
        //else {
        //    console.log("IntersectionObserver is disabled");
        //}
    }
};
NETDATA.intersectionObserver.init();

// ----------------------------------------------------------------------------------------------------------------
// Our state object, where all per-chart values are stored

let chartState = function (element) {
    this.element = element;

    // IMPORTANT:
    // all private functions should use 'that', instead of 'this'
    // Alternatively, you can use arrow functions (related issue #4514)
    let that = this;

    // ============================================================================================================
    // ERROR HANDLING

    /* error() - private
     * show an error instead of the chart
     */
    let error = (msg) => {
        let ret = true;

        if (typeof netdataErrorCallback === 'function') {
            ret = netdataErrorCallback('chart', this.id, msg);
        }

        if (ret) {
            this.element.innerHTML = this.id + ': ' + msg;
            this.enabled = false;
            this.current = this.pan;
        }
    };

    // console logging
    this.log = function (msg) {
        console.log(this.id + ' (' + this.library_name + ' ' + this.uuid + '): ' + msg);
    };

    this.debugLog = function (msg) {
        if (this.debug) {
            this.log(msg);
        }
    };

    // ============================================================================================================
    // EARLY INITIALIZATION

    // These are variables that should exist even if the chart is never to be rendered.
    // Be careful what you add here - there may be thousands of charts on the page.

    // GUID - a unique identifier for the chart
    this.uuid = NETDATA.guid();

    // string - the name of chart
    this.id = NETDATA.dataAttribute(this.element, 'netdata', undefined);
    if (typeof this.id === 'undefined') {
        error("netdata elements need data-netdata");
        return;
    }

    // string - the key for localStorage settings
    this.settings_id = NETDATA.dataAttribute(this.element, 'id', null);

    // the user given dimensions of the element
    this.width = NETDATA.dataAttribute(this.element, 'width', NETDATA.chartDefaults.width);
    this.height = NETDATA.dataAttribute(this.element, 'height', NETDATA.chartDefaults.height);
    this.height_original = this.height;

    if (this.settings_id !== null) {
        this.height = NETDATA.localStorageGet('chart_heights.' + this.settings_id, this.height, function (height) {
            // this is the callback that will be called
            // if and when the user resets all localStorage variables
            // to their defaults

            resizeChartToHeight(height);
        });
    }

    // the chart library requested by the user
    this.library_name = NETDATA.dataAttribute(this.element, 'chart-library', NETDATA.chartDefaults.library);

    // check the requested library is available
    // we don't initialize it here - it will be initialized when
    // this chart will be first used
    if (typeof NETDATA.chartLibraries[this.library_name] === 'undefined') {
        NETDATA.error(402, this.library_name);
        error('chart library "' + this.library_name + '" is not found');
        this.enabled = false;
    } else if (!NETDATA.chartLibraries[this.library_name].enabled) {
        NETDATA.error(403, this.library_name);
        error('chart library "' + this.library_name + '" is not enabled');
        this.enabled = false;
    } else {
        this.library = NETDATA.chartLibraries[this.library_name];
    }

    this.auto = {
        name: 'auto',
        autorefresh: true,
        force_update_at: 0, // the timestamp to force the update at
        force_before_ms: null,
        force_after_ms: null
    };
    this.pan = {
        name: 'pan',
        autorefresh: false,
        force_update_at: 0, // the timestamp to force the update at
        force_before_ms: null,
        force_after_ms: null
    };
    this.zoom = {
        name: 'zoom',
        autorefresh: false,
        force_update_at: 0, // the timestamp to force the update at
        force_before_ms: null,
        force_after_ms: null
    };

    // this is a pointer to one of the sub-classes below
    // auto, pan, zoom
    this.current = this.auto;

    this.running = false;                       // boolean - true when the chart is being refreshed now
    this.enabled = true;                        // boolean - is the chart enabled for refresh?

    this.force_update_every = null;             // number - overwrite the visualization update frequency of the chart

    this.tmp = {};

    this.foreignElementBefore = null;
    this.foreignElementAfter = null;
    this.foreignElementDuration = null;
    this.foreignElementUpdateEvery = null;
    this.foreignElementSelection = null;

    // ============================================================================================================
    // PRIVATE FUNCTIONS

    // reset the runtime status variables to their defaults
    const runtimeInit = () => {
        this.paused = false;                        // boolean - is the chart paused for any reason?
        this.selected = false;                      // boolean - is the chart shown a selection?

        this.chart_created = false;                 // boolean - is the library.create() been called?
        this.dom_created = false;                   // boolean - is the chart DOM been created?
        this.fetching_data = false;                 // boolean - true while we fetch data via ajax

        this.updates_counter = 0;                   // numeric - the number of refreshes made so far
        this.updates_since_last_unhide = 0;         // numeric - the number of refreshes made since the last time the chart was unhidden
        this.updates_since_last_creation = 0;       // numeric - the number of refreshes made since the last time the chart was created

        this.tm = {
            last_initialized: 0,                    // milliseconds - the timestamp it was last initialized
            last_dom_created: 0,                    // milliseconds - the timestamp its DOM was last created
            last_mode_switch: 0,                    // milliseconds - the timestamp it switched modes

            last_info_downloaded: 0,                // milliseconds - the timestamp we downloaded the chart
            last_updated: 0,                        // the timestamp the chart last updated with data
            pan_and_zoom_seq: 0,                    // the sequence number of the global synchronization
                                                    // between chart.
                                                    // Used with NETDATA.globalPanAndZoom.seq
            last_visible_check: 0,                  // the time we last checked if it is visible
            last_resized: 0,                        // the time the chart was resized
            last_hidden: 0,                         // the time the chart was hidden
            last_unhidden: 0,                       // the time the chart was unhidden
            last_autorefreshed: 0                   // the time the chart was last refreshed
        };

        this.data = null;                           // the last data as downloaded from the netdata server
        this.data_url = 'invalid://';               // string - the last url used to update the chart
        this.data_points = 0;                       // number - the number of points returned from netdata
        this.data_after = 0;                        // milliseconds - the first timestamp of the data
        this.data_before = 0;                       // milliseconds - the last timestamp of the data
        this.data_update_every = 0;                 // milliseconds - the frequency to update the data

        this.tmp = {};                              // members that can be destroyed to save memory
    };

    // initialize all the variables that are required for the chart to be rendered
    const lateInitialization = () => {
        if (typeof this.host !== 'undefined') {
            return;
        }

        // string - the netdata server URL, without any path
        this.host = NETDATA.dataAttribute(this.element, 'host', NETDATA.serverDefault);

        // make sure the host does not end with /
        // all netdata API requests use absolute paths
        while (this.host.slice(-1) === '/') {
            this.host = this.host.substring(0, this.host.length - 1);
        }

        // string - the grouping method requested by the user
        this.method = NETDATA.dataAttribute(this.element, 'method', NETDATA.chartDefaults.method);
        this.gtime = NETDATA.dataAttribute(this.element, 'gtime', 0);

        // the time-range requested by the user
        this.after = NETDATA.dataAttribute(this.element, 'after', NETDATA.chartDefaults.after);
        this.before = NETDATA.dataAttribute(this.element, 'before', NETDATA.chartDefaults.before);

        // the pixels per point requested by the user
        this.pixels_per_point = NETDATA.dataAttribute(this.element, 'pixels-per-point', 1);
        this.points = NETDATA.dataAttribute(this.element, 'points', null);

        // the forced update_every
        this.force_update_every = NETDATA.dataAttribute(this.element, 'update-every', null);
        if (typeof this.force_update_every !== 'number' || this.force_update_every <= 1) {
            if (this.force_update_every !== null) {
                this.log('ignoring invalid value of property data-update-every');
            }

            this.force_update_every = null;
        } else {
            this.force_update_every *= 1000;
        }

        // the dimensions requested by the user
        this.dimensions = NETDATA.encodeURIComponent(NETDATA.dataAttribute(this.element, 'dimensions', null));

        this.title = NETDATA.dataAttribute(this.element, 'title', null);    // the title of the chart
        this.units = NETDATA.dataAttribute(this.element, 'units', null);    // the units of the chart dimensions
        this.units_desired = NETDATA.dataAttribute(this.element, 'desired-units', NETDATA.options.current.units); // the units of the chart dimensions
        this.units_current = this.units;
        this.units_common = NETDATA.dataAttribute(this.element, 'common-units', null);

        // additional options to pass to netdata
        this.append_options = NETDATA.encodeURIComponent(NETDATA.dataAttribute(this.element, 'append-options', null));

        // override options to pass to netdata
        this.override_options = NETDATA.encodeURIComponent(NETDATA.dataAttribute(this.element, 'override-options', null));

        this.debug = NETDATA.dataAttributeBoolean(this.element, 'debug', false);

        this.value_decimal_detail = -1;
        let d = NETDATA.dataAttribute(this.element, 'decimal-digits', -1);
        if (typeof d === 'number') {
            this.value_decimal_detail = d;
        } else if (typeof d !== 'undefined') {
            this.log('ignoring decimal-digits value: ' + d.toString());
        }

        // if we need to report the rendering speed
        // find the element that needs to be updated
        let refresh_dt_element_name = NETDATA.dataAttribute(this.element, 'dt-element-name', null); // string - the element to print refresh_dt_ms

        if (refresh_dt_element_name !== null) {
            this.refresh_dt_element = document.getElementById(refresh_dt_element_name) || null;
        }
        else {
            this.refresh_dt_element = null;
        }

        this.dimensions_visibility = new dimensionsVisibility(that);

        this.netdata_first = 0;                     // milliseconds - the first timestamp in netdata
        this.netdata_last = 0;                      // milliseconds - the last timestamp in netdata
        this.requested_after = null;                // milliseconds - the timestamp of the request after param
        this.requested_before = null;               // milliseconds - the timestamp of the request before param
        this.requested_padding = null;
        this.view_after = 0;
        this.view_before = 0;

        this.refresh_dt_ms = 0;                     // milliseconds - the time the last refresh took

        // how many retries we have made to load chart data from the server
        this.retries_on_data_failures = 0;

        // color management
        this.colors = null;
        this.colors_assigned = null;
        this.colors_available = null;
        this.colors_custom = null;

        this.element_message = null; // the element already created by the user
        this.element_chart = null; // the element with the chart
        this.element_legend = null; // the element with the legend of the chart (if created by us)
        this.element_legend_childs = {
            content: null,
            hidden: null,
            title_date: null,
            title_time: null,
            title_units: null,
            perfect_scroller: null, // the container to apply perfect scroller to
            series: null
        };

        this.chart_url = null;                      // string - the url to download chart info
        this.chart = null;                          // object - the chart as downloaded from the server

        const getForeignElementById = (opt) => {
            let id = NETDATA.dataAttribute(this.element, opt, null);
            if (id === null) {
                //this.log('option "' + opt + '" is undefined');
                return null;
            }

            let el = document.getElementById(id);
            if (typeof el === 'undefined') {
                this.log('cannot find an element with name "' + id.toString() + '"');
                return null;
            }

            return el;
        };

        this.foreignElementBefore = getForeignElementById('show-before-at');
        this.foreignElementAfter = getForeignElementById('show-after-at');
        this.foreignElementDuration = getForeignElementById('show-duration-at');
        this.foreignElementUpdateEvery = getForeignElementById('show-update-every-at');
        this.foreignElementSelection = getForeignElementById('show-selection-at');
    };

    const destroyDOM = () => {
        if (!this.enabled) {
            return;
        }

        if (this.debug) {
            this.log('destroyDOM()');
        }

        // this.element.className = 'netdata-message icon';
        // this.element.innerHTML = '<i class="fas fa-sync"></i> netdata';
        this.element.innerHTML = '';
        this.element_message = null;
        this.element_legend = null;
        this.element_chart = null;
        this.element_legend_childs.series = null;

        this.chart_created = false;
        this.dom_created = false;

        this.tm.last_resized = 0;
        this.tm.last_dom_created = 0;
    };

    let createDOM = () => {
        if (!this.enabled) {
            return;
        }
        lateInitialization();

        destroyDOM();

        if (this.debug) {
            this.log('createDOM()');
        }

        this.element_message = document.createElement('div');
        this.element_message.className = 'netdata-message icon hidden';
        this.element.appendChild(this.element_message);

        this.dom_created = true;
        this.chart_created = false;

        this.tm.last_dom_created = this.tm.last_resized = Date.now();

        showLoading();
    };

    const initDOM = () => {
        this.element.className = this.library.container_class(that);

        if (typeof(this.width) === 'string') {
            this.element.style.width = this.width;
        } else if (typeof(this.width) === 'number') {
            this.element.style.width = this.width.toString() + 'px';
        }

        if (typeof(this.library.aspect_ratio) === 'undefined') {
            if (typeof(this.height) === 'string') {
                this.element.style.height = this.height;
            } else if (typeof(this.height) === 'number') {
                this.element.style.height = this.height.toString() + 'px';
            }
        }

        if (NETDATA.chartDefaults.min_width !== null) {
            this.element.style.min_width = NETDATA.chartDefaults.min_width;
        }
    };

    const invisibleSearchableText = () => {
        return '<span style="position:absolute; opacity: 0; width: 0px;">' + this.id + '</span>';
    };

    /* init() private
     * initialize state variables
     * destroy all (possibly) created state elements
     * create the basic DOM for a chart
     */
    const init = (opt) => {
        if (!this.enabled) {
            return;
        }

        runtimeInit();
        this.element.innerHTML = invisibleSearchableText();

        this.tm.last_initialized = Date.now();
        this.setMode('auto');

        if (opt !== 'fast') {
            if (this.isVisible(true) || opt === 'force') {
                createDOM();
            }
        }
    };

    const maxMessageFontSize = () => {
        let screenHeight = screen.height;
        let el = this.element;

        // normally we want a font size, as tall as the element
        let h = el.clientHeight;

        // but give it some air, 20% let's say, or 5 pixels min
        let lost = Math.max(h * 0.2, 5);
        h -= lost;

        // center the text, vertically
        let paddingTop = (lost - 5) / 2;

        // but check the width too
        // it should fit 10 characters in it
        let w = el.clientWidth / 10;
        if (h > w) {
            paddingTop += (h - w) / 2;
            h = w;
        }

        // and don't make it too huge
        // 5% of the screen size is good
        if (h > screenHeight / 20) {
            paddingTop += (h - (screenHeight / 20)) / 2;
            h = screenHeight / 20;
        }

        // set it
        this.element_message.style.fontSize = h.toString() + 'px';
        this.element_message.style.paddingTop = paddingTop.toString() + 'px';
    };

    const showMessageIcon = (icon) => {
        this.element_message.innerHTML = icon;
        maxMessageFontSize();
        $(this.element_message).removeClass('hidden');
        this.tmp.___messageHidden___ = undefined;
    };

    const hideMessage = () => {
        if (typeof this.tmp.___messageHidden___ === 'undefined') {
            this.tmp.___messageHidden___ = true;
            $(this.element_message).addClass('hidden');
        }
    };

    const showRendering = () => {
        let icon;
        if (this.chart !== null) {
            if (this.chart.chart_type === 'line') {
                icon = NETDATA.icons.lineChart;
            } else {
                icon = NETDATA.icons.areaChart;
            }
        }
        else {
            icon = NETDATA.icons.noChart;
        }

        showMessageIcon(icon + ' netdata' + invisibleSearchableText());
    };

    const showLoading = () => {
        if (!this.chart_created) {
            showMessageIcon(NETDATA.icons.loading + ' netdata');
            return true;
        }
        return false;
    };

    const isHidden = () => {
        return (typeof this.tmp.___chartIsHidden___ !== 'undefined');
    };

    // hide the chart, when it is not visible - called from isVisible()
    this.hideChart = function () {
        // hide it, if it is not already hidden
        if (isHidden()) {
            return;
        }

        if (this.chart_created) {
            if (NETDATA.options.current.show_help) {
                if (this.element_legend_childs.toolbox !== null) {
                    if (this.debug) {
                        this.log('hideChart(): hidding legend popovers');
                    }

                    $(this.element_legend_childs.toolbox_left).popover('hide');
                    $(this.element_legend_childs.toolbox_reset).popover('hide');
                    $(this.element_legend_childs.toolbox_right).popover('hide');
                    $(this.element_legend_childs.toolbox_zoomin).popover('hide');
                    $(this.element_legend_childs.toolbox_zoomout).popover('hide');
                }

                if (this.element_legend_childs.resize_handler !== null) {
                    $(this.element_legend_childs.resize_handler).popover('hide');
                }

                if (this.element_legend_childs.content !== null) {
                    $(this.element_legend_childs.content).popover('hide');
                }
            }

            if (NETDATA.options.current.destroy_on_hide) {
                if (this.debug) {
                    this.log('hideChart(): initializing chart');
                }

                // we should destroy it
                init('force');
            } else {
                if (this.debug) {
                    this.log('hideChart(): hiding chart');
                }

                showRendering();
                this.element_chart.style.display = 'none';
                this.element.style.willChange = 'auto';
                if (this.element_legend !== null) {
                    this.element_legend.style.display = 'none';
                }
                if (this.element_legend_childs.toolbox !== null) {
                    this.element_legend_childs.toolbox.style.display = 'none';
                }
                if (this.element_legend_childs.resize_handler !== null) {
                    this.element_legend_childs.resize_handler.style.display = 'none';
                }

                this.tm.last_hidden = Date.now();

                // de-allocate data
                // This works, but I not sure there are no corner cases somewhere
                // so it is commented - if the user has memory issues he can
                // set Destroy on Hide for all charts
                // this.data = null;
            }
        }

        this.tmp.___chartIsHidden___ = true;
    };

    // unhide the chart, when it is visible - called from isVisible()
    this.unhideChart = function () {
        if (!isHidden()) {
            return;
        }

        this.tmp.___chartIsHidden___ = undefined;
        this.updates_since_last_unhide = 0;

        if (!this.chart_created) {
            if (this.debug) {
                this.log('unhideChart(): initializing chart');
            }

            // we need to re-initialize it, to show our background
            // logo in bootstrap tabs, until the chart loads
            init('force');
        } else {
            if (this.debug) {
                this.log('unhideChart(): unhiding chart');
            }

            this.element.style.willChange = 'transform';
            this.tm.last_unhidden = Date.now();
            this.element_chart.style.display = '';
            if (this.element_legend !== null) {
                this.element_legend.style.display = '';
            }
            if (this.element_legend_childs.toolbox !== null) {
                this.element_legend_childs.toolbox.style.display = '';
            }
            if (this.element_legend_childs.resize_handler !== null) {
                this.element_legend_childs.resize_handler.style.display = '';
            }
            resizeChart();
            hideMessage();
        }

        if (this.__redraw_on_unhide) {
            if (this.debug) {
                this.log("redrawing chart on unhide");
            }

            this.__redraw_on_unhide = undefined;
            this.redrawChart();
        }
    };

    const canBeRendered = (uncached_visibility) => {
        if (this.debug) {
            this.log('canBeRendered() called');
        }

        if (!NETDATA.options.current.update_only_visible) {
            return true;
        }

        let ret = (
            (
                NETDATA.options.page_is_visible ||
                NETDATA.options.current.stop_updates_when_focus_is_lost === false ||
                this.updates_since_last_unhide === 0
            )
            && isHidden() === false && this.isVisible(uncached_visibility)
        );

        if (this.debug) {
            this.log('canBeRendered(): ' + ret);
        }

        return ret;
    };

    // https://github.com/petkaantonov/bluebird/wiki/Optimization-killers
    const callChartLibraryUpdateSafely = (data) => {
        let status;

        // we should not do this here
        // if we prevent rendering the chart then:
        // 1. globalSelectionSync will be wrong
        // 2. globalPanAndZoom will be wrong
        //if (canBeRendered(true) === false)
        //    return false;

        if (NETDATA.options.fake_chart_rendering) {
            return true;
        }

        this.updates_counter++;
        this.updates_since_last_unhide++;
        this.updates_since_last_creation++;

        if (NETDATA.options.debug.chart_errors) {
            status = this.library.update(that, data);
        } else {
            try {
                status = this.library.update(that, data);
            } catch (err) {
                status = false;
            }
        }

        if (!status) {
            error('chart failed to be updated as ' + this.library_name);
            return false;
        }

        return true;
    };

    // https://github.com/petkaantonov/bluebird/wiki/Optimization-killers
    const callChartLibraryCreateSafely = (data) => {
        let status;

        // we should not do this here
        // if we prevent rendering the chart then:
        // 1. globalSelectionSync will be wrong
        // 2. globalPanAndZoom will be wrong
        //if (canBeRendered(true) === false)
        //    return false;

        if (NETDATA.options.fake_chart_rendering) {
            return true;
        }

        this.updates_counter++;
        this.updates_since_last_unhide++;
        this.updates_since_last_creation++;

        if (NETDATA.options.debug.chart_errors) {
            status = this.library.create(that, data);
        } else {
            try {
                status = this.library.create(that, data);
            } catch (err) {
                status = false;
            }
        }

        if (!status) {
            error('chart failed to be created as ' + this.library_name);
            return false;
        }

        this.chart_created = true;
        this.updates_since_last_creation = 0;
        return true;
    };

    // ----------------------------------------------------------------------------------------------------------------
    // Chart Resize

    // resizeChart() - private
    // to be called just before the chart library to make sure that
    // a properly sized dom is available
    const resizeChart = () => {
        if (this.tm.last_resized < NETDATA.options.last_page_resize) {
            if (!this.chart_created) {
                return;
            }

            if (this.needsRecreation()) {
                if (this.debug) {
                    this.log('resizeChart(): initializing chart');
                }

                init('force');
            } else if (typeof this.library.resize === 'function') {
                if (this.debug) {
                    this.log('resizeChart(): resizing chart');
                }

                this.library.resize(that);

                if (this.element_legend_childs.perfect_scroller !== null) {
                    Ps.update(this.element_legend_childs.perfect_scroller);
                }

                maxMessageFontSize();
            }

            this.tm.last_resized = Date.now();
        }
    };

    // this is the actual chart resize algorithm
    // it will:
    // - resize the entire container
    // - update the internal states
    // - resize the chart as the div changes height
    // - update the scrollbar of the legend
    const resizeChartToHeight = (h) => {
        // console.log(h);
        this.element.style.height = h;

        if (this.settings_id !== null) {
            NETDATA.localStorageSet('chart_heights.' + this.settings_id, h);
        }

        let now = Date.now();
        NETDATA.options.last_page_scroll = now;
        NETDATA.options.auto_refresher_stop_until = now + NETDATA.options.current.stop_updates_while_resizing;

        // force a resize
        this.tm.last_resized = 0;
        resizeChart();
    };

    this.resizeForPrint = function () {
        if (typeof this.element_legend_childs !== 'undefined' && this.element_legend_childs.perfect_scroller !== null) {
            let current = this.element.clientHeight;
            let optimal = current
                + this.element_legend_childs.perfect_scroller.scrollHeight
                - this.element_legend_childs.perfect_scroller.clientHeight;

            if (optimal > current) {
                // this.log('resized');
                this.element.style.height = optimal + 'px';
                this.library.resize(this);
            }
        }
    };

    this.resizeHandler = function (e) {
        e.preventDefault();

        if (typeof this.event_resize === 'undefined'
            || this.event_resize.chart_original_w === 'undefined'
            || this.event_resize.chart_original_h === 'undefined') {
            this.event_resize = {
                chart_original_w: this.element.clientWidth,
                chart_original_h: this.element.clientHeight,
                last: 0
            };
        }

        if (e.type === 'touchstart') {
            this.event_resize.mouse_start_x = e.touches.item(0).pageX;
            this.event_resize.mouse_start_y = e.touches.item(0).pageY;
        } else {
            this.event_resize.mouse_start_x = e.clientX;
            this.event_resize.mouse_start_y = e.clientY;
        }

        this.event_resize.chart_start_w = this.element.clientWidth;
        this.event_resize.chart_start_h = this.element.clientHeight;
        this.event_resize.chart_last_w = this.element.clientWidth;
        this.event_resize.chart_last_h = this.element.clientHeight;

        let now = Date.now();
        if (now - this.event_resize.last <= NETDATA.options.current.double_click_speed && this.element_legend_childs.perfect_scroller !== null) {
            // double click / double tap event

            // console.dir(this.element_legend_childs.content);
            // console.dir(this.element_legend_childs.perfect_scroller);

            // the optimal height of the chart
            // showing the entire legend
            let optimal = this.event_resize.chart_last_h
                + this.element_legend_childs.perfect_scroller.scrollHeight
                - this.element_legend_childs.perfect_scroller.clientHeight;

            // if we are not optimal, be optimal
            if (this.event_resize.chart_last_h !== optimal) {
                // this.log('resize to optimal, current = ' + this.event_resize.chart_last_h.toString() + 'px, original = ' + this.event_resize.chart_original_h.toString() + 'px, optimal = ' + optimal.toString() + 'px, internal = ' + this.height_original.toString());
                resizeChartToHeight(optimal.toString() + 'px');
            }

            // else if the current height is not the original/saved height
            // reset to the original/saved height
            else if (this.event_resize.chart_last_h !== this.event_resize.chart_original_h) {
                // this.log('resize to original, current = ' + this.event_resize.chart_last_h.toString() + 'px, original = ' + this.event_resize.chart_original_h.toString() + 'px, optimal = ' + optimal.toString() + 'px, internal = ' + this.height_original.toString());
                resizeChartToHeight(this.event_resize.chart_original_h.toString() + 'px');
            }

            // else if the current height is not the internal default height
            // reset to the internal default height
            else if ((this.event_resize.chart_last_h.toString() + 'px') !== this.height_original) {
                // this.log('resize to internal default, current = ' + this.event_resize.chart_last_h.toString() + 'px, original = ' + this.event_resize.chart_original_h.toString() + 'px, optimal = ' + optimal.toString() + 'px, internal = ' + this.height_original.toString());
                resizeChartToHeight(this.height_original.toString());
            }

            // else if the current height is not the firstchild's clientheight
            // resize to it
            else if (typeof this.element_legend_childs.perfect_scroller.firstChild !== 'undefined') {
                let parent_rect = this.element.getBoundingClientRect();
                let content_rect = this.element_legend_childs.perfect_scroller.firstElementChild.getBoundingClientRect();
                let wanted = content_rect.top - parent_rect.top + this.element_legend_childs.perfect_scroller.firstChild.clientHeight + 18; // 15 = toolbox + 3 space

                // console.log(parent_rect);
                // console.log(content_rect);
                // console.log(wanted);

                // this.log('resize to firstChild, current = ' + this.event_resize.chart_last_h.toString() + 'px, original = ' + this.event_resize.chart_original_h.toString() + 'px, optimal = ' + optimal.toString() + 'px, internal = ' + this.height_original.toString() + 'px, firstChild = ' + wanted.toString() + 'px' );
                if (this.event_resize.chart_last_h !== wanted) {
                    resizeChartToHeight(wanted.toString() + 'px');
                }
            }
        } else {
            this.event_resize.last = now;

            // process movement event
            document.onmousemove =
                document.ontouchmove =
                    this.element_legend_childs.resize_handler.onmousemove =
                        this.element_legend_childs.resize_handler.ontouchmove =
                            function (e) {
                                let y = null;

                                switch (e.type) {
                                    case 'mousemove':
                                        y = e.clientY;
                                        break;
                                    case 'touchmove':
                                        y = e.touches.item(e.touches - 1).pageY;
                                        break;
                                }

                                if (y !== null) {
                                    let newH = that.event_resize.chart_start_h + y - that.event_resize.mouse_start_y;

                                    if (newH >= 70 && newH !== that.event_resize.chart_last_h) {
                                        resizeChartToHeight(newH.toString() + 'px');
                                        that.event_resize.chart_last_h = newH;
                                    }
                                }
                            };

            // process end event
            document.onmouseup =
                document.ontouchend =
                    this.element_legend_childs.resize_handler.onmouseup =
                        this.element_legend_childs.resize_handler.ontouchend =
                            function (e) {
                                void(e);

                                // remove all the hooks
                                document.onmouseup =
                                    document.onmousemove =
                                        document.ontouchmove =
                                            document.ontouchend =
                                                that.element_legend_childs.resize_handler.onmousemove =
                                                    that.element_legend_childs.resize_handler.ontouchmove =
                                                        that.element_legend_childs.resize_handler.onmouseout =
                                                            that.element_legend_childs.resize_handler.onmouseup =
                                                                that.element_legend_childs.resize_handler.ontouchend =
                                                                    null;

                                // allow auto-refreshes
                                NETDATA.options.auto_refresher_stop_until = 0;
                            };
        }
    };

    const noDataToShow = () => {
        showMessageIcon(NETDATA.icons.noData + ' empty');
        this.legendUpdateDOM();
        this.tm.last_autorefreshed = Date.now();
        // this.data_update_every = 30 * 1000;
        //this.element_chart.style.display = 'none';
        //if (this.element_legend !== null) this.element_legend.style.display = 'none';
        //this.tmp.___chartIsHidden___ = true;
    };

    // ============================================================================================================
    // PUBLIC FUNCTIONS

    this.error = function (msg) {
        error(msg);
    };

    this.setMode = function (m) {
        if (this.current !== null && this.current.name === m) {
            return;
        }

        if (m === 'auto') {
            this.current = this.auto;
        } else if (m === 'pan') {
            this.current = this.pan;
        } else if (m === 'zoom') {
            this.current = this.zoom;
        } else {
            this.current = this.auto;
        }

        this.current.force_update_at = 0;
        this.current.force_before_ms = null;
        this.current.force_after_ms = null;

        this.tm.last_mode_switch = Date.now();
    };

    // ----------------------------------------------------------------------------------------------------------------
    // global selection sync for slaves

    // can the chart participate to the global selection sync as a slave?
    this.globalSelectionSyncIsEligible = function () {
        return (
            this.enabled &&
            this.library !== null &&
            typeof this.library.setSelection === 'function' &&
            this.isVisible() &&
            this.chart_created
        );
    };

    this.setSelection = function (t) {
        if (typeof this.library.setSelection === 'function') {
            // this.selected = this.library.setSelection(this, t) === true;
            this.selected = this.library.setSelection(this, t);
        } else {
            this.selected = true;
        }

        if (this.selected && this.debug) {
            this.log('selection set to ' + t.toString());
        }

        if (this.foreignElementSelection !== null) {
            this.foreignElementSelection.innerText = NETDATA.dateTime.localeDateString(t) + ' ' + NETDATA.dateTime.localeTimeString(t);
        }

        return this.selected;
    };

    this.clearSelection = function () {
        if (this.selected) {
            if (typeof this.library.clearSelection === 'function') {
                this.selected = (this.library.clearSelection(this) !== true);
            } else {
                this.selected = false;
            }

            if (this.selected === false && this.debug) {
                this.log('selection cleared');
            }

            if (this.foreignElementSelection !== null) {
                this.foreignElementSelection.innerText = '';
            }

            this.legendReset();
        }

        return this.selected;
    };

    // ----------------------------------------------------------------------------------------------------------------

    // find if a timestamp (ms) is shown in the current chart
    this.timeIsVisible = function (t) {
        return (t >= this.data_after && t <= this.data_before);
    };

    this.calculateRowForTime = function (t) {
        if (!this.timeIsVisible(t)) {
            return -1;
        }
        return Math.floor((t - this.data_after) / this.data_update_every);
    };

    // ----------------------------------------------------------------------------------------------------------------

    this.pauseChart = function () {
        if (!this.paused) {
            if (this.debug) {
                this.log('pauseChart()');
            }

            this.paused = true;
        }
    };

    this.unpauseChart = function () {
        if (this.paused) {
            if (this.debug) {
                this.log('unpauseChart()');
            }

            this.paused = false;
        }
    };

    this.resetChart = function (dontClearMaster, dontUpdate) {
        if (this.debug) {
            this.log('resetChart(' + dontClearMaster + ', ' + dontUpdate + ') called');
        }

        if (typeof dontClearMaster === 'undefined') {
            dontClearMaster = false;
        }

        if (typeof dontUpdate === 'undefined') {
            dontUpdate = false;
        }

        if (dontClearMaster !== true && NETDATA.globalPanAndZoom.isMaster(this)) {
            if (this.debug) {
                this.log('resetChart() diverting to clearMaster().');
            }
            // this will call us back with master === true
            NETDATA.globalPanAndZoom.clearMaster();
            return;
        }

        this.clearSelection();

        this.tm.pan_and_zoom_seq = 0;

        this.setMode('auto');
        this.current.force_update_at = 0;
        this.current.force_before_ms = null;
        this.current.force_after_ms = null;
        this.tm.last_autorefreshed = 0;
        this.paused = false;
        this.selected = false;
        this.enabled = true;
        // this.debug = false;

        // do not update the chart here
        // or the chart will flip-flop when it is the master
        // of a selection sync and another chart becomes
        // the new master

        if (dontUpdate !== true && this.isVisible()) {
            this.updateChart();
        }
    };

    this.updateChartPanOrZoom = function (after, before, callback) {
        let logme = 'updateChartPanOrZoom(' + after + ', ' + before + '): ';
        let ret = true;

        NETDATA.globalPanAndZoom.delay();
        NETDATA.globalSelectionSync.delay();

        if (this.debug) {
            this.log(logme);
        }

        if (before < after) {
            if (this.debug) {
                this.log(logme + 'flipped parameters, rejecting it.');
            }
            return false;
        }

        if (typeof this.fixed_min_duration === 'undefined') {
            this.fixed_min_duration = Math.round((this.chartWidth() / 30) * this.chart.update_every * 1000);
        }

        let min_duration = this.fixed_min_duration;
        let current_duration = Math.round(this.view_before - this.view_after);

        // round the numbers
        after = Math.round(after);
        before = Math.round(before);

        // align them to update_every
        // stretching them further away
        after -= after % this.data_update_every;
        before += this.data_update_every - (before % this.data_update_every);

        // the final wanted duration
        let wanted_duration = before - after;

        // to allow panning, accept just a point below our minimum
        if ((current_duration - this.data_update_every) < min_duration) {
            min_duration = current_duration - this.data_update_every;
        }

        // we do it, but we adjust to minimum size and return false
        // when the wanted size is below the current and the minimum
        // and we zoom
        if (wanted_duration < current_duration && wanted_duration < min_duration) {
            if (this.debug) {
                this.log(logme + 'too small: min_duration: ' + (min_duration / 1000).toString() + ', wanted: ' + (wanted_duration / 1000).toString());
            }

            min_duration = this.fixed_min_duration;

            let dt = (min_duration - wanted_duration) / 2;
            before += dt;
            after -= dt;
            wanted_duration = before - after;
            ret = false;
        }

        let tolerance = this.data_update_every * 2;
        let movement = Math.abs(before - this.view_before);

        if (Math.abs(current_duration - wanted_duration) <= tolerance && movement <= tolerance && ret) {
            if (this.debug) {
                this.log(logme + 'REJECTING UPDATE: current/min duration: ' + (current_duration / 1000).toString() + '/' + (this.fixed_min_duration / 1000).toString() + ', wanted duration: ' + (wanted_duration / 1000).toString() + ', duration diff: ' + (Math.round(Math.abs(current_duration - wanted_duration) / 1000)).toString() + ', movement: ' + (movement / 1000).toString() + ', tolerance: ' + (tolerance / 1000).toString() + ', returning: ' + false);
            }
            return false;
        }

        if (this.current.name === 'auto') {
            this.log(logme + 'caller called me with mode: ' + this.current.name);
            this.setMode('pan');
        }

        if (this.debug) {
            this.log(logme + 'ACCEPTING UPDATE: current/min duration: ' + (current_duration / 1000).toString() + '/' + (this.fixed_min_duration / 1000).toString() + ', wanted duration: ' + (wanted_duration / 1000).toString() + ', duration diff: ' + (Math.round(Math.abs(current_duration - wanted_duration) / 1000)).toString() + ', movement: ' + (movement / 1000).toString() + ', tolerance: ' + (tolerance / 1000).toString() + ', returning: ' + ret);
        }

        this.current.force_update_at = Date.now() + NETDATA.options.current.pan_and_zoom_delay;
        this.current.force_after_ms = after;
        this.current.force_before_ms = before;
        NETDATA.globalPanAndZoom.setMaster(this, after, before);

        if (ret && typeof callback === 'function') {
            callback();
        }

        return ret;
    };

    this.updateChartPanOrZoomAsyncTimeOutId = undefined;
    this.updateChartPanOrZoomAsync = function (after, before, callback) {
        NETDATA.globalPanAndZoom.delay();
        NETDATA.globalSelectionSync.delay();

        if (!NETDATA.globalPanAndZoom.isMaster(this)) {
            this.pauseChart();
            NETDATA.globalPanAndZoom.setMaster(this, after, before);
            // NETDATA.globalSelectionSync.stop();
            NETDATA.globalSelectionSync.setMaster(this);
        }

        if (this.updateChartPanOrZoomAsyncTimeOutId) {
            NETDATA.timeout.clear(this.updateChartPanOrZoomAsyncTimeOutId);
        }

        NETDATA.timeout.set(function () {
            that.updateChartPanOrZoomAsyncTimeOutId = undefined;
            that.updateChartPanOrZoom(after, before, callback);
        }, 0);
    };

    let _unitsConversionLastUnits = undefined;
    let _unitsConversionLastUnitsDesired = undefined;
    let _unitsConversionLastMin = undefined;
    let _unitsConversionLastMax = undefined;
    let _unitsConversion = function (value) {
        return value;
    };
    this.unitsConversionSetup = function (min, max) {
        if (this.units !== _unitsConversionLastUnits
            || this.units_desired !== _unitsConversionLastUnitsDesired
            || min !== _unitsConversionLastMin
            || max !== _unitsConversionLastMax) {

            _unitsConversionLastUnits = this.units;
            _unitsConversionLastUnitsDesired = this.units_desired;
            _unitsConversionLastMin = min;
            _unitsConversionLastMax = max;

            _unitsConversion = NETDATA.unitsConversion.get(this.uuid, min, max, this.units, this.units_desired, this.units_common, function (units) {
                // console.log('switching units from ' + that.units.toString() + ' to ' + units.toString());
                that.units_current = units;
                that.legendSetUnitsString(that.units_current);
            });
        }
    };

    let _legendFormatValueChartDecimalsLastMin = undefined;
    let _legendFormatValueChartDecimalsLastMax = undefined;
    let _legendFormatValueChartDecimals = -1;
    let _intlNumberFormat = null;
    this.legendFormatValueDecimalsFromMinMax = function (min, max) {
        if (min === _legendFormatValueChartDecimalsLastMin && max === _legendFormatValueChartDecimalsLastMax) {
            return;
        }

        this.unitsConversionSetup(min, max);
        if (_unitsConversion !== null) {
            min = _unitsConversion(min);
            max = _unitsConversion(max);

            if (typeof min !== 'number' || typeof max !== 'number') {
                return;
            }
        }

        _legendFormatValueChartDecimalsLastMin = min;
        _legendFormatValueChartDecimalsLastMax = max;

        let old = _legendFormatValueChartDecimals;

        if (this.data !== null && this.data.min === this.data.max)
        // it is a fixed number, let the visualizer decide based on the value
        {
            _legendFormatValueChartDecimals = -1;
        } else if (this.value_decimal_detail !== -1)
        // there is an override
        {
            _legendFormatValueChartDecimals = this.value_decimal_detail;
        } else {
            // ok, let's calculate the proper number of decimal points
            let delta;

            if (min === max) {
                delta = Math.abs(min);
            } else {
                delta = Math.abs(max - min);
            }

            if (delta > 1000) {
                _legendFormatValueChartDecimals = 0;
            } else if (delta > 10) {
                _legendFormatValueChartDecimals = 1;
            } else if (delta > 1) {
                _legendFormatValueChartDecimals = 2;
            } else if (delta > 0.1) {
                _legendFormatValueChartDecimals = 2;
            } else if (delta > 0.01) {
                _legendFormatValueChartDecimals = 4;
            } else if (delta > 0.001) {
                _legendFormatValueChartDecimals = 5;
            } else if (delta > 0.0001) {
                _legendFormatValueChartDecimals = 6;
            } else {
                _legendFormatValueChartDecimals = 7;
            }
        }

        if (_legendFormatValueChartDecimals !== old) {
            if (_legendFormatValueChartDecimals < 0) {
                _intlNumberFormat = null;
            } else {
                _intlNumberFormat = NETDATA.fastNumberFormat.get(
                    _legendFormatValueChartDecimals,
                    _legendFormatValueChartDecimals
                );
            }
        }
    };

    this.legendFormatValue = function (value) {
        if (typeof value !== 'number') {
            return '-';
        }

        value = _unitsConversion(value);

        if (typeof value !== 'number') {
            return value;
        }

        if (_intlNumberFormat !== null) {
            return _intlNumberFormat.format(value);
        }

        let dmin, dmax;
        if (this.value_decimal_detail !== -1) {
            dmin = dmax = this.value_decimal_detail;
        } else {
            dmin = 0;
            let abs = (value < 0) ? -value : value;
            if (abs > 1000) {
                dmax = 0;
            } else if (abs > 10) {
                dmax = 1;
            } else if (abs > 1) {
                dmax = 2;
            } else if (abs > 0.1) {
                dmax = 2;
            } else if (abs > 0.01) {
                dmax = 4;
            } else if (abs > 0.001) {
                dmax = 5;
            } else if (abs > 0.0001) {
                dmax = 6;
            } else {
                dmax = 7;
            }
        }

        return NETDATA.fastNumberFormat.get(dmin, dmax).format(value);
    };

    this.legendSetLabelValue = function (label, value) {
        let series = this.element_legend_childs.series[label];
        if (typeof series === 'undefined') {
            return;
        }
        if (series.value === null && series.user === null) {
            return;
        }

        /*
        // this slows down firefox and edge significantly
        // since it requires to use innerHTML(), instead of innerText()

        // if the value has not changed, skip DOM update
        //if (series.last === value) return;

        let s, r;
        if (typeof value === 'number') {
            let v = Math.abs(value);
            s = r = this.legendFormatValue(value);

            if (typeof series.last === 'number') {
                if (v > series.last) s += '<i class="fas fa-angle-up" style="width: 8px; text-align: center; overflow: hidden; vertical-align: middle;"></i>';
                else if (v < series.last) s += '<i class="fas fa-angle-down" style="width: 8px; text-align: center; overflow: hidden; vertical-align: middle;"></i>';
                else s += '<i class="fas fa-angle-left" style="width: 8px; text-align: center; overflow: hidden; vertical-align: middle;"></i>';
            }
            else s += '<i class="fas fa-angle-right" style="width: 8px; text-align: center; overflow: hidden; vertical-align: middle;"></i>';

            series.last = v;
        }
        else {
            if (value === null)
                s = r = '';
            else
                s = r = value;

            series.last = value;
        }
        */

        let s = this.legendFormatValue(value);

        // caching: do not update the update to show the same value again
        if (s === series.last_shown_value) {
            return;
        }
        series.last_shown_value = s;

        if (series.value !== null) {
            series.value.innerText = s;
        }
        if (series.user !== null) {
            series.user.innerText = s;
        }
    };

    this.legendSetDateString = function (date) {
        if (this.element_legend_childs.title_date !== null && date !== this.tmp.__last_shown_legend_date) {
            this.element_legend_childs.title_date.innerText = date;
            this.tmp.__last_shown_legend_date = date;
        }
    };

    this.legendSetTimeString = function (time) {
        if (this.element_legend_childs.title_time !== null && time !== this.tmp.__last_shown_legend_time) {
            this.element_legend_childs.title_time.innerText = time;
            this.tmp.__last_shown_legend_time = time;
        }
    };

    this.legendSetUnitsString = function (units) {
        if (this.element_legend_childs.title_units !== null && units !== this.tmp.__last_shown_legend_units) {
            this.element_legend_childs.title_units.innerText = units;
            this.tmp.__last_shown_legend_units = units;
        }
    };

    this.legendSetDateLast = {
        ms: 0,
        date: undefined,
        time: undefined
    };

    this.legendSetDate = function (ms) {
        if (typeof ms !== 'number') {
            this.legendShowUndefined();
            return;
        }

        if (this.legendSetDateLast.ms !== ms) {
            let d = new Date(ms);
            this.legendSetDateLast.ms = ms;
            this.legendSetDateLast.date = NETDATA.dateTime.localeDateString(d);
            this.legendSetDateLast.time = NETDATA.dateTime.localeTimeString(d);
        }

        this.legendSetDateString(this.legendSetDateLast.date);
        this.legendSetTimeString(this.legendSetDateLast.time);
        this.legendSetUnitsString(this.units_current)
    };

    this.legendShowUndefined = function () {
        this.legendSetDateString(this.legendPluginModuleString(false));
        this.legendSetTimeString(this.chart.context.toString());
        // this.legendSetUnitsString(' ');

        if (this.data && this.element_legend_childs.series !== null) {
            let labels = this.data.dimension_names;
            let i = labels.length;
            while (i--) {
                let label = labels[i];

                if (typeof label === 'undefined' || typeof this.element_legend_childs.series[label] === 'undefined') {
                    continue;
                }
                this.legendSetLabelValue(label, null);
            }
        }
    };

    this.legendShowLatestValues = function () {
        if (this.chart === null) {
            return;
        }
        if (this.selected) {
            return;
        }

        if (this.data === null || this.element_legend_childs.series === null) {
            this.legendShowUndefined();
            return;
        }

        let show_undefined = true;
        if (Math.abs(this.netdata_last - this.view_before) <= this.data_update_every) {
            show_undefined = false;
        }

        if (show_undefined) {
            this.legendShowUndefined();
            return;
        }

        this.legendSetDate(this.view_before);

        let labels = this.data.dimension_names;
        let i = labels.length;
        while (i--) {
            let label = labels[i];

            if (typeof label === 'undefined') {
                continue;
            }
            if (typeof this.element_legend_childs.series[label] === 'undefined') {
                continue;
            }

            this.legendSetLabelValue(label, this.data.view_latest_values[i]);
        }
    };

    this.legendReset = function () {
        this.legendShowLatestValues();
    };

    // this should be called just ONCE per dimension per chart
    this.__chartDimensionColor = function (label) {
        let c = NETDATA.commonColors.get(this, label);

        // it is important to maintain a list of colors
        // for this chart only, since the chart library
        // uses this to assign colors to dimensions in the same
        // order the dimension are given to it
        this.colors.push(c);

        return c;
    };

    this.chartPrepareColorPalette = function () {
        NETDATA.commonColors.refill(this);
    };

    // get the ordered list of chart colors
    // this includes user defined colors
    this.chartCustomColors = function () {
        this.chartPrepareColorPalette();

        let colors;
        if (this.colors_custom.length) {
            colors = this.colors_custom;
        } else {
            colors = this.colors;
        }

        if (this.debug) {
            this.log("chartCustomColors() returns:");
            this.log(colors);
        }

        return colors;
    };

    // get the ordered list of chart ASSIGNED colors
    // (this returns only the colors that have been
    //  assigned to dimensions, prepended with any
    // custom colors defined)
    this.chartColors = function () {
        this.chartPrepareColorPalette();

        if (this.debug) {
            this.log("chartColors() returns:");
            this.log(this.colors);
        }

        return this.colors;
    };

    this.legendPluginModuleString = function (withContext) {
        let str = ' ';
        let context = '';

        if (typeof this.chart !== 'undefined') {
            if (withContext && typeof this.chart.context === 'string') {
                context = this.chart.context;
            }

            if (typeof this.chart.plugin === 'string' && this.chart.plugin !== '') {
                str = this.chart.plugin;

                if (str.endsWith(".plugin")) {
                    str = str.substring(0, str.length - 7);
                }

                if (typeof this.chart.module === 'string' && this.chart.module !== '') {
                    str += ':' + this.chart.module;
                }

                if (withContext && context !== '') {
                    str += ', ' + context;
                }
            }
            else if (withContext && context !== '') {
                str = context;
            }
        }

        return str;
    };

    this.legendResolutionTooltip = function () {
        if (!this.chart) {
            return '';
        }

        let collected = this.chart.update_every;
        let viewed = (this.data) ? this.data.view_update_every : collected;

        if (collected === viewed) {
            return "resolution " + NETDATA.seconds4human(collected);
        }

        return "resolution " + NETDATA.seconds4human(viewed) + ", collected every " + NETDATA.seconds4human(collected);
    };

    this.legendUpdateDOM = function () {
        let needed = false, dim, keys, len;

        // check that the legend DOM is up to date for the downloaded dimensions
        if (typeof this.element_legend_childs.series !== 'object' || this.element_legend_childs.series === null) {
            // this.log('the legend does not have any series - requesting legend update');
            needed = true;
        } else if (this.data === null) {
            // this.log('the chart does not have any data - requesting legend update');
            needed = true;
        } else if (typeof this.element_legend_childs.series.labels_key === 'undefined') {
            needed = true;
        } else {
            let labels = this.data.dimension_names.toString();
            if (labels !== this.element_legend_childs.series.labels_key) {
                needed = true;

                if (this.debug) {
                    this.log('NEW LABELS: "' + labels + '" NOT EQUAL OLD LABELS: "' + this.element_legend_childs.series.labels_key + '"');
                }
            }
        }

        if (!needed) {
            // make sure colors available
            this.chartPrepareColorPalette();

            // do we have to update the current values?
            // we do this, only when the visible chart is current
            if (Math.abs(this.netdata_last - this.view_before) <= this.data_update_every) {
                if (this.debug) {
                    this.log('chart is in latest position... updating values on legend...');
                }

                //let labels = this.data.dimension_names;
                //let i = labels.length;
                //while (i--)
                //  this.legendSetLabelValue(labels[i], this.data.view_latest_values[i]);
            }
            return;
        }

        if (this.colors === null) {
            // this is the first time we update the chart
            // let's assign colors to all dimensions
            if (this.library.track_colors()) {
                this.colors = [];
                keys = Object.keys(this.chart.dimensions);
                len = keys.length;
                for (let i = 0; i < len; i++) {
                    NETDATA.commonColors.get(this, this.chart.dimensions[keys[i]].name);
                }
            }
        }

        // we will re-generate the colors for the chart
        // based on the dimensions this result has data for
        this.colors = [];

        if (this.debug) {
            this.log('updating Legend DOM');
        }

        // mark all dimensions as invalid
        this.dimensions_visibility.invalidateAll();

        const genLabel = function (state, parent, dim, name, count) {
            let color = state.__chartDimensionColor(name);

            let user_element = null;
            let user_id = NETDATA.dataAttribute(state.element, 'show-value-of-' + name.toLowerCase() + '-at', null);
            if (user_id === null) {
                user_id = NETDATA.dataAttribute(state.element, 'show-value-of-' + dim.toLowerCase() + '-at', null);
            }
            if (user_id !== null) {
                user_element = document.getElementById(user_id) || null;
                if (user_element === null) {
                    state.log('Cannot find element with id: ' + user_id);
                }
            }

            state.element_legend_childs.series[name] = {
                name: document.createElement('span'),
                value: document.createElement('span'),
                user: user_element,
                last: null,
                last_shown_value: null
            };

            let label = state.element_legend_childs.series[name];

            // create the dimension visibility tracking for this label
            state.dimensions_visibility.dimensionAdd(name, label.name, label.value, color);

            let rgb = NETDATA.colorHex2Rgb(color);
            label.name.innerHTML = '<table class="netdata-legend-name-table-'
                + state.chart.chart_type
                + '" style="background-color: '
                + 'rgba(' + rgb.r + ',' + rgb.g + ',' + rgb.b + ',' + NETDATA.options.current['color_fill_opacity_' + state.chart.chart_type] + ') !important'
                + '"><tr class="netdata-legend-name-tr"><td class="netdata-legend-name-td"></td></tr></table>';

            let text = document.createTextNode(' ' + name);
            label.name.appendChild(text);

            if (count > 0) {
                parent.appendChild(document.createElement('br'));
            }

            parent.appendChild(label.name);
            parent.appendChild(label.value);
        };

        let content = document.createElement('div');

        if (this.element_chart === null) {
            this.element_chart = document.createElement('div');
            this.element_chart.id = this.library_name + '-' + this.uuid + '-chart';
            this.element.appendChild(this.element_chart);

            if (this.hasLegend()) {
                this.element_chart.className = 'netdata-chart-with-legend-right netdata-' + this.library_name + '-chart-with-legend-right';
            } else {
                this.element_chart.className = ' netdata-chart netdata-' + this.library_name + '-chart';
            }
        }

        if (this.hasLegend()) {
            if (this.element_legend === null) {
                this.element_legend = document.createElement('div');
                this.element_legend.className = 'netdata-chart-legend netdata-' + this.library_name + '-legend';
                this.element.appendChild(this.element_legend);
            } else {
                this.element_legend.innerHTML = '';
            }

            this.element_legend_childs = {
                content: content,
                resize_handler: null,
                toolbox: null,
                toolbox_left: null,
                toolbox_right: null,
                toolbox_reset: null,
                toolbox_zoomin: null,
                toolbox_zoomout: null,
                toolbox_volume: null,
                title_date: document.createElement('span'),
                title_time: document.createElement('span'),
                title_units: document.createElement('span'),
                perfect_scroller: document.createElement('div'),
                series: {}
            };

            if (NETDATA.options.current.legend_toolbox && this.library.toolboxPanAndZoom !== null) {
                this.element_legend_childs.toolbox = document.createElement('div');
                this.element_legend_childs.toolbox_left = document.createElement('div');
                this.element_legend_childs.toolbox_right = document.createElement('div');
                this.element_legend_childs.toolbox_reset = document.createElement('div');
                this.element_legend_childs.toolbox_zoomin = document.createElement('div');
                this.element_legend_childs.toolbox_zoomout = document.createElement('div');
                this.element_legend_childs.toolbox_volume = document.createElement('div');

                const getPanAndZoomStep = function (event) {
                    if (event.ctrlKey) {
                        return NETDATA.options.current.pan_and_zoom_factor * NETDATA.options.current.pan_and_zoom_factor_multiplier_control;
                    } else if (event.shiftKey) {
                        return NETDATA.options.current.pan_and_zoom_factor * NETDATA.options.current.pan_and_zoom_factor_multiplier_shift;
                    } else if (event.altKey) {
                        return NETDATA.options.current.pan_and_zoom_factor * NETDATA.options.current.pan_and_zoom_factor_multiplier_alt;
                    } else {
                        return NETDATA.options.current.pan_and_zoom_factor;
                    }
                };

                this.element_legend_childs.toolbox.className += ' netdata-legend-toolbox';
                this.element.appendChild(this.element_legend_childs.toolbox);

                this.element_legend_childs.toolbox_left.className += ' netdata-legend-toolbox-button';
                this.element_legend_childs.toolbox_left.innerHTML = NETDATA.icons.left;
                this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_left);
                this.element_legend_childs.toolbox_left.onclick = function (e) {
                    e.preventDefault();

                    let step = (that.view_before - that.view_after) * getPanAndZoomStep(e);
                    let before = that.view_before - step;
                    let after = that.view_after - step;
                    if (after >= that.netdata_first) {
                        that.library.toolboxPanAndZoom(that, after, before);
                    }
                };
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.toolbox_left).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Pan Left',
                        content: 'Pan the chart to the left. You can also <b>drag it</b> with your mouse or your finger (on touch devices).<br/><small>Help can be disabled from the settings.</small>'
                    });
                }

                this.element_legend_childs.toolbox_reset.className += ' netdata-legend-toolbox-button';
                this.element_legend_childs.toolbox_reset.innerHTML = NETDATA.icons.reset;
                this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_reset);
                this.element_legend_childs.toolbox_reset.onclick = function (e) {
                    e.preventDefault();
                    NETDATA.resetAllCharts(that);
                };
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.toolbox_reset).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Chart Reset',
                        content: 'Reset all the charts to their default auto-refreshing state. You can also <b>double click</b> the chart contents with your mouse or your finger (on touch devices).<br/><small>Help can be disabled from the settings.</small>'
                    });
                }

                this.element_legend_childs.toolbox_right.className += ' netdata-legend-toolbox-button';
                this.element_legend_childs.toolbox_right.innerHTML = NETDATA.icons.right;
                this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_right);
                this.element_legend_childs.toolbox_right.onclick = function (e) {
                    e.preventDefault();
                    let step = (that.view_before - that.view_after) * getPanAndZoomStep(e);
                    let before = that.view_before + step;
                    let after = that.view_after + step;
                    if (before <= that.netdata_last) {
                        that.library.toolboxPanAndZoom(that, after, before);
                    }
                };
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.toolbox_right).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Pan Right',
                        content: 'Pan the chart to the right. You can also <b>drag it</b> with your mouse or your finger (on touch devices).<br/><small>Help, can be disabled from the settings.</small>'
                    });
                }

                this.element_legend_childs.toolbox_zoomin.className += ' netdata-legend-toolbox-button';
                this.element_legend_childs.toolbox_zoomin.innerHTML = NETDATA.icons.zoomIn;
                this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_zoomin);
                this.element_legend_childs.toolbox_zoomin.onclick = function (e) {
                    e.preventDefault();
                    let dt = ((that.view_before - that.view_after) * (getPanAndZoomStep(e) * 0.8) / 2);
                    let before = that.view_before - dt;
                    let after = that.view_after + dt;
                    that.library.toolboxPanAndZoom(that, after, before);
                };
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.toolbox_zoomin).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Chart Zoom In',
                        content: 'Zoom in the chart. You can also press SHIFT and select an area of the chart, or press SHIFT or ALT and use the mouse wheel or 2-finger touchpad scroll to zoom in or out.<br/><small>Help, can be disabled from the settings.</small>'
                    });
                }

                this.element_legend_childs.toolbox_zoomout.className += ' netdata-legend-toolbox-button';
                this.element_legend_childs.toolbox_zoomout.innerHTML = NETDATA.icons.zoomOut;
                this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_zoomout);
                this.element_legend_childs.toolbox_zoomout.onclick = function (e) {
                    e.preventDefault();
                    let dt = (((that.view_before - that.view_after) / (1.0 - (getPanAndZoomStep(e) * 0.8)) - (that.view_before - that.view_after)) / 2);
                    let before = that.view_before + dt;
                    let after = that.view_after - dt;

                    that.library.toolboxPanAndZoom(that, after, before);
                };
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.toolbox_zoomout).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Chart Zoom Out',
                        content: 'Zoom out the chart. You can also press SHIFT or ALT and use the mouse wheel, or 2-finger touchpad scroll to zoom in or out.<br/><small>Help, can be disabled from the settings.</small>'
                    });
                }

                //this.element_legend_childs.toolbox_volume.className += ' netdata-legend-toolbox-button';
                //this.element_legend_childs.toolbox_volume.innerHTML = '<i class="fas fa-sort-amount-down"></i>';
                //this.element_legend_childs.toolbox_volume.title = 'Visible Volume';
                //this.element_legend_childs.toolbox.appendChild(this.element_legend_childs.toolbox_volume);
                //this.element_legend_childs.toolbox_volume.onclick = function(e) {
                //e.preventDefault();
                //alert('clicked toolbox_volume on ' + that.id);
                //}
            }

            if (NETDATA.options.current.resize_charts) {
                this.element_legend_childs.resize_handler = document.createElement('div');

                this.element_legend_childs.resize_handler.className += " netdata-legend-resize-handler";
                this.element_legend_childs.resize_handler.innerHTML = NETDATA.icons.resize;
                this.element.appendChild(this.element_legend_childs.resize_handler);
                if (NETDATA.options.current.show_help) {
                    $(this.element_legend_childs.resize_handler).popover({
                        container: "body",
                        animation: false,
                        html: true,
                        trigger: 'hover',
                        placement: 'bottom',
                        delay: {
                            show: NETDATA.options.current.show_help_delay_show_ms,
                            hide: NETDATA.options.current.show_help_delay_hide_ms
                        },
                        title: 'Chart Resize',
                        content: 'Drag this point with your mouse or your finger (on touch devices), to resize the chart vertically. You can also <b>double click it</b> or <b>double tap it</b> to reset between 2 states: the default and the one that fits all the values.<br/><small>Help, can be disabled from the settings.</small>'
                    });
                }

                // mousedown event
                this.element_legend_childs.resize_handler.onmousedown =
                    function (e) {
                        that.resizeHandler(e);
                    };

                // touchstart event
                this.element_legend_childs.resize_handler.addEventListener('touchstart', function (e) {
                    that.resizeHandler(e);
                }, false);
            }

            if (this.chart) {
                this.element_legend_childs.title_date.title = this.legendPluginModuleString(true);
                this.element_legend_childs.title_time.title = this.legendResolutionTooltip();
            }

            this.element_legend_childs.title_date.className += " netdata-legend-title-date";
            this.element_legend.appendChild(this.element_legend_childs.title_date);
            this.tmp.__last_shown_legend_date = undefined;

            this.element_legend.appendChild(document.createElement('br'));

            this.element_legend_childs.title_time.className += " netdata-legend-title-time";
            this.element_legend.appendChild(this.element_legend_childs.title_time);
            this.tmp.__last_shown_legend_time = undefined;

            this.element_legend.appendChild(document.createElement('br'));

            this.element_legend_childs.title_units.className += " netdata-legend-title-units";
            this.element_legend_childs.title_units.innerText = this.units_current;
            this.element_legend.appendChild(this.element_legend_childs.title_units);
            this.tmp.__last_shown_legend_units = undefined;

            this.element_legend.appendChild(document.createElement('br'));

            this.element_legend_childs.perfect_scroller.className = 'netdata-legend-series';
            this.element_legend.appendChild(this.element_legend_childs.perfect_scroller);

            content.className = 'netdata-legend-series-content';
            this.element_legend_childs.perfect_scroller.appendChild(content);

            this.element_legend_childs.content = content;

            if (NETDATA.options.current.show_help) {
                $(content).popover({
                    container: "body",
                    animation: false,
                    html: true,
                    trigger: 'hover',
                    placement: 'bottom',
                    title: 'Chart Legend',
                    delay: {
                        show: NETDATA.options.current.show_help_delay_show_ms,
                        hide: NETDATA.options.current.show_help_delay_hide_ms
                    },
                    content: 'You can click or tap on the values or the labels to select dimensions. By pressing SHIFT or CONTROL, you can enable or disable multiple dimensions.<br/><small>Help, can be disabled from the settings.</small>'
                });
            }
        } else {
            this.element_legend_childs = {
                content: content,
                resize_handler: null,
                toolbox: null,
                toolbox_left: null,
                toolbox_right: null,
                toolbox_reset: null,
                toolbox_zoomin: null,
                toolbox_zoomout: null,
                toolbox_volume: null,
                title_date: null,
                title_time: null,
                title_units: null,
                perfect_scroller: null,
                series: {}
            };
        }

        if (this.data) {
            this.element_legend_childs.series.labels_key = this.data.dimension_names.toString();
            if (this.debug) {
                this.log('labels from data: "' + this.element_legend_childs.series.labels_key + '"');
            }

            for (let i = 0, len = this.data.dimension_names.length; i < len; i++) {
                genLabel(this, content, this.data.dimension_ids[i], this.data.dimension_names[i], i);
            }
        } else {
            let tmp = [];
            keys = Object.keys(this.chart.dimensions);
            for (let i = 0, len = keys.length; i < len; i++) {
                dim = keys[i];
                tmp.push(this.chart.dimensions[dim].name);
                genLabel(this, content, dim, this.chart.dimensions[dim].name, i);
            }
            this.element_legend_childs.series.labels_key = tmp.toString();
            if (this.debug) {
                this.log('labels from chart: "' + this.element_legend_childs.series.labels_key + '"');
            }
        }

        // create a hidden div to be used for hidding
        // the original legend of the chart library
        let el = document.createElement('div');
        if (this.element_legend !== null) {
            this.element_legend.appendChild(el);
        }
        el.style.display = 'none';

        this.element_legend_childs.hidden = document.createElement('div');
        el.appendChild(this.element_legend_childs.hidden);

        if (this.element_legend_childs.perfect_scroller !== null) {
            Ps.initialize(this.element_legend_childs.perfect_scroller, {
                wheelSpeed: 0.2,
                wheelPropagation: true,
                swipePropagation: true,
                minScrollbarLength: null,
                maxScrollbarLength: null,
                useBothWheelAxes: false,
                suppressScrollX: true,
                suppressScrollY: false,
                scrollXMarginOffset: 0,
                scrollYMarginOffset: 0,
                theme: 'default'
            });
            Ps.update(this.element_legend_childs.perfect_scroller);
        }

        this.legendShowLatestValues();
    };

    this.hasLegend = function () {
        if (typeof this.tmp.___hasLegendCache___ !== 'undefined') {
            return this.tmp.___hasLegendCache___;
        }

        let leg = false;
        if (this.library && this.library.legend(this) === 'right-side') {
            leg = true;
        }

        this.tmp.___hasLegendCache___ = leg;
        return leg;
    };

    this.legendWidth = function () {
        return (this.hasLegend()) ? 140 : 0;
    };

    this.legendHeight = function () {
        return $(this.element).height();
    };

    this.chartWidth = function () {
        return $(this.element).width() - this.legendWidth();
    };

    this.chartHeight = function () {
        return $(this.element).height();
    };

    this.chartPixelsPerPoint = function () {
        // force an options provided detail
        let px = this.pixels_per_point;

        if (this.library && px < this.library.pixels_per_point(this)) {
            px = this.library.pixels_per_point(this);
        }

        if (px < NETDATA.options.current.pixels_per_point) {
            px = NETDATA.options.current.pixels_per_point;
        }

        return px;
    };

    this.needsRecreation = function () {
        let ret = (
            this.chart_created &&
            this.library &&
            this.library.autoresize() === false &&
            this.tm.last_resized < NETDATA.options.last_page_resize
        );

        if (this.debug) {
            this.log('needsRecreation(): ' + ret.toString() + ', chart_created = ' + this.chart_created.toString());
        }

        return ret;
    };

    this.chartDataUniqueID = function () {
        return this.id + ',' + this.library_name + ',' + this.dimensions + ',' + this.chartURLOptions();
    };

    this.chartURLOptions = function () {
        let ret = '';

        if (this.override_options !== null) {
            ret = this.override_options.toString();
        } else {
            ret = this.library.options(this);
        }

        if (this.append_options !== null) {
            ret += '%7C' + this.append_options.toString();
        }

        ret += '%7C' + 'jsonwrap';

        if (NETDATA.options.current.eliminate_zero_dimensions) {
            ret += '%7C' + 'nonzero';
        }

        return ret;
    };

    this.chartURL = function () {
        let after, before, points_multiplier = 1;
        if (NETDATA.globalPanAndZoom.isActive()) {
            if (this.current.force_before_ms !== null && this.current.force_after_ms !== null) {
                this.tm.pan_and_zoom_seq = 0;

                before = Math.round(this.current.force_before_ms / 1000);
                after = Math.round(this.current.force_after_ms / 1000);
                this.view_after = after * 1000;
                this.view_before = before * 1000;

                if (NETDATA.options.current.pan_and_zoom_data_padding) {
                    this.requested_padding = Math.round((before - after) / 2);
                    after -= this.requested_padding;
                    before += this.requested_padding;
                    this.requested_padding *= 1000;
                    points_multiplier = 2;
                }

                this.current.force_before_ms = null;
                this.current.force_after_ms = null;
            } else {
                this.tm.pan_and_zoom_seq = NETDATA.globalPanAndZoom.seq;

                after = Math.round(NETDATA.globalPanAndZoom.force_after_ms / 1000);
                before = Math.round(NETDATA.globalPanAndZoom.force_before_ms / 1000);
                this.view_after = after * 1000;
                this.view_before = before * 1000;

                this.requested_padding = null;
                points_multiplier = 1;
            }
        } else {
            this.tm.pan_and_zoom_seq = 0;

            before = this.before;
            after = this.after;
            this.view_after = after * 1000;
            this.view_before = before * 1000;

            this.requested_padding = null;
            points_multiplier = 1;
        }

        this.requested_after = after * 1000;
        this.requested_before = before * 1000;

        let data_points;
        if (NETDATA.options.force_data_points !== 0) {
            data_points = NETDATA.options.force_data_points;
            this.data_points = data_points;
        } else {
            this.data_points = this.points || Math.round(this.chartWidth() / this.chartPixelsPerPoint());
            data_points = this.data_points * points_multiplier;
        }

        // build the data URL
        this.data_url = this.host + this.chart.data_url;
        this.data_url += "&format=" + this.library.format();
        this.data_url += "&points=" + (data_points).toString();
        this.data_url += "&group=" + this.method;
        this.data_url += "&gtime=" + this.gtime;
        this.data_url += "&options=" + this.chartURLOptions();

        if (after) {
            this.data_url += "&after=" + after.toString();
        }

        if (before) {
            this.data_url += "&before=" + before.toString();
        }

        if (this.dimensions) {
            this.data_url += "&dimensions=" + this.dimensions;
        }

        if (NETDATA.options.debug.chart_data_url || this.debug) {
            this.log('chartURL(): ' + this.data_url + ' WxH:' + this.chartWidth() + 'x' + this.chartHeight() + ' points: ' + data_points.toString() + ' library: ' + this.library_name);
        }
    };

    this.redrawChart = function () {
        if (this.data !== null) {
            this.updateChartWithData(this.data);
        }
    };

    this.updateChartWithData = function (data) {
        if (this.debug) {
            this.log('updateChartWithData() called.');
        }

        // this may force the chart to be re-created
        resizeChart();

        this.data = data;

        let started = Date.now();
        let view_update_every = data.view_update_every * 1000;

        if (this.data_update_every !== view_update_every) {
            if (this.element_legend_childs.title_time) {
                this.element_legend_childs.title_time.title = this.legendResolutionTooltip();
            }
        }

        // if the result is JSON, find the latest update-every
        this.data_update_every = view_update_every;
        this.data_after = data.after * 1000;
        this.data_before = data.before * 1000;
        this.netdata_first = data.first_entry * 1000;
        this.netdata_last = data.last_entry * 1000;
        this.data_points = data.points;

        data.state = this;

        if (NETDATA.options.current.pan_and_zoom_data_padding && this.requested_padding !== null) {
            if (this.view_after < this.data_after) {
                // console.log('adjusting view_after from ' + this.view_after + ' to ' + this.data_after);
                this.view_after = this.data_after;
            }

            if (this.view_before > this.data_before) {
                // console.log('adjusting view_before from ' + this.view_before + ' to ' + this.data_before);
                this.view_before = this.data_before;
            }
        } else {
            this.view_after = this.data_after;
            this.view_before = this.data_before;
        }

        if (this.debug) {
            this.log('UPDATE No ' + this.updates_counter + ' COMPLETED');

            if (this.current.force_after_ms) {
                this.log('STATUS: forced    : ' + (this.current.force_after_ms / 1000).toString() + ' - ' + (this.current.force_before_ms / 1000).toString());
            } else {
                this.log('STATUS: forced    : unset');
            }

            this.log('STATUS: requested : ' + (this.requested_after / 1000).toString() + ' - ' + (this.requested_before / 1000).toString());
            this.log('STATUS: downloaded: ' + (this.data_after / 1000).toString() + ' - ' + (this.data_before / 1000).toString());
            this.log('STATUS: rendered  : ' + (this.view_after / 1000).toString() + ' - ' + (this.view_before / 1000).toString());
            this.log('STATUS: points    : ' + (this.data_points).toString());
        }

        if (this.data_points === 0) {
            noDataToShow();
            return;
        }

        if (this.updates_since_last_creation >= this.library.max_updates_to_recreate()) {
            if (this.debug) {
                this.log('max updates of ' + this.updates_since_last_creation.toString() + ' reached. Forcing re-generation.');
            }

            init('force');
            return;
        }

        // check and update the legend
        this.legendUpdateDOM();

        if (this.chart_created && typeof this.library.update === 'function') {
            if (this.debug) {
                this.log('updating chart...');
            }

            if (!callChartLibraryUpdateSafely(data)) {
                return;
            }
        } else {
            if (this.debug) {
                this.log('creating chart...');
            }

            if (!callChartLibraryCreateSafely(data)) {
                return;
            }
        }

        if (this.isVisible()) {
            hideMessage();
            this.legendShowLatestValues();
        } else {
            this.__redraw_on_unhide = true;

            if (this.debug) {
                this.log("drawn while not visible");
            }
        }

        if (this.selected) {
            NETDATA.globalSelectionSync.stop();
        }

        // update the performance counters
        let now = Date.now();
        this.tm.last_updated = now;

        // don't update last_autorefreshed if this chart is
        // forced to be updated with global PanAndZoom
        if (NETDATA.globalPanAndZoom.isActive()) {
            this.tm.last_autorefreshed = 0;
        } else {
            if (NETDATA.options.current.parallel_refresher && NETDATA.options.current.concurrent_refreshes && typeof this.force_update_every !== 'number') {
                this.tm.last_autorefreshed = now - (now % this.data_update_every);
            } else {
                this.tm.last_autorefreshed = now;
            }
        }

        this.refresh_dt_ms = now - started;
        NETDATA.options.auto_refresher_fast_weight += this.refresh_dt_ms;

        if (this.refresh_dt_element !== null) {
            this.refresh_dt_element.innerText = this.refresh_dt_ms.toString();
        }

        if (this.foreignElementBefore !== null) {
            this.foreignElementBefore.innerText = NETDATA.dateTime.localeDateString(this.view_before) + ' ' + NETDATA.dateTime.localeTimeString(this.view_before);
        }

        if (this.foreignElementAfter !== null) {
            this.foreignElementAfter.innerText = NETDATA.dateTime.localeDateString(this.view_after) + ' ' + NETDATA.dateTime.localeTimeString(this.view_after);
        }

        if (this.foreignElementDuration !== null) {
            this.foreignElementDuration.innerText = NETDATA.seconds4human(Math.floor((this.view_before - this.view_after) / 1000) + 1);
        }

        if (this.foreignElementUpdateEvery !== null) {
            this.foreignElementUpdateEvery.innerText = NETDATA.seconds4human(Math.floor(this.data_update_every / 1000));
        }
    };

    this.getSnapshotData = function (key) {
        if (this.debug) {
            this.log('updating from snapshot: ' + key);
        }

        if (typeof netdataSnapshotData.data[key] === 'undefined') {
            this.log('snapshot does not include data for key "' + key + '"');
            return null;
        }

        if (typeof netdataSnapshotData.data[key] !== 'string') {
            this.log('snapshot data for key "' + key + '" is not string');
            return null;
        }

        let uncompressed;
        try {
            uncompressed = netdataSnapshotData.uncompress(netdataSnapshotData.data[key]);

            if (uncompressed === null) {
                this.log('uncompressed snapshot data for key ' + key + ' is null');
                return null;
            }

            if (typeof uncompressed === 'undefined') {
                this.log('uncompressed snapshot data for key ' + key + ' is undefined');
                return null;
            }
        } catch (e) {
            this.log('decompression of snapshot data for key ' + key + ' failed');
            console.log(e);
            uncompressed = null;
        }

        if (typeof uncompressed !== 'string') {
            this.log('uncompressed snapshot data for key ' + key + ' is not string');
            return null;
        }

        let data;
        try {
            data = JSON.parse(uncompressed);
        } catch (e) {
            this.log('parsing snapshot data for key ' + key + ' failed');
            console.log(e);
            data = null;
        }

        return data;
    };

    this.updateChart = function (callback) {
        if (this.debug) {
            this.log('updateChart()');
        }

        if (this.fetching_data) {
            if (this.debug) {
                this.log('updateChart(): I am already updating...');
            }

            if (typeof callback === 'function') {
                return callback(false, 'already running');
            }

            return;
        }

        // due to late initialization of charts and libraries
        // we need to check this too
        if (!this.enabled) {
            if (this.debug) {
                this.log('updateChart(): I am not enabled');
            }

            if (typeof callback === 'function') {
                return callback(false, 'not enabled');
            }

            return;
        }

        if (!canBeRendered()) {
            if (this.debug) {
                this.log('updateChart(): cannot be rendered');
            }

            if (typeof callback === 'function') {
                return callback(false, 'cannot be rendered');
            }

            return;
        }

        if (that.dom_created !== true) {
            if (this.debug) {
                this.log('updateChart(): creating DOM');
            }

            createDOM();
        }

        if (this.chart === null) {
            if (this.debug) {
                this.log('updateChart(): getting chart');
            }

            return this.getChart(function () {
                return that.updateChart(callback);
            });
        }

        if (!this.library.initialized) {
            if (this.library.enabled) {
                if (this.debug) {
                    this.log('updateChart(): initializing chart library');
                }

                return this.library.initialize(function () {
                    return that.updateChart(callback);
                });
            } else {
                error('chart library "' + this.library_name + '" is not available.');

                if (typeof callback === 'function') {
                    return callback(false, 'library not available');
                }

                return;
            }
        }

        this.clearSelection();
        this.chartURL();

        NETDATA.statistics.refreshes_total++;
        NETDATA.statistics.refreshes_active++;

        if (NETDATA.statistics.refreshes_active > NETDATA.statistics.refreshes_active_max) {
            NETDATA.statistics.refreshes_active_max = NETDATA.statistics.refreshes_active;
        }

        let ok = false;
        this.fetching_data = true;

        if (netdataSnapshotData !== null) {
            let key = this.chartDataUniqueID();
            let data = this.getSnapshotData(key);
            if (data !== null) {
                ok = true;
                data = NETDATA.xss.checkData('/api/v1/data', data, this.library.xssRegexIgnore);
                this.updateChartWithData(data);
            } else {
                ok = false;
                error('cannot get data from snapshot for key: "' + key + '"');
                that.tm.last_autorefreshed = Date.now();
            }

            NETDATA.statistics.refreshes_active--;
            this.fetching_data = false;

            if (typeof callback === 'function') {
                callback(ok, 'snapshot');
            }

            return;
        }

        if (this.debug) {
            this.log('updating from ' + this.data_url);
        }

        this.xhr = $.ajax({
            url: this.data_url,
            cache: false,
            async: true,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkData('/api/v1/data', data, that.library.xssRegexIgnore);

                that.xhr = undefined;
                that.retries_on_data_failures = 0;
                ok = true;

                if (that.debug) {
                    that.log('data received. updating chart.');
                }

                that.updateChartWithData(data);
            })
            .fail(function (msg) {
                that.xhr = undefined;

                if (msg.statusText !== 'abort') {
                    that.retries_on_data_failures++;
                    if (that.retries_on_data_failures > NETDATA.options.current.retries_on_data_failures) {
                        // that.log('failed ' + that.retries_on_data_failures.toString() + ' times - giving up');
                        that.retries_on_data_failures = 0;
                        error('data download failed for url: ' + that.data_url);
                    }
                    else {
                        that.tm.last_autorefreshed = Date.now();
                        // that.log('failed ' + that.retries_on_data_failures.toString() + ' times, but I will retry');
                    }
                }
            })
            .always(function () {
                that.xhr = undefined;

                NETDATA.statistics.refreshes_active--;
                that.fetching_data = false;

                if (typeof callback === 'function') {
                    return callback(ok, 'download');
                }
            });
    };

    const __isVisible = function () {
        let ret = true;

        if (NETDATA.options.current.update_only_visible !== false) {
            // tolerance is the number of pixels a chart can be off-screen
            // to consider it as visible and refresh it as if was visible
            let tolerance = 0;

            that.tm.last_visible_check = Date.now();

            let rect = that.element.getBoundingClientRect();

            let screenTop = window.scrollY;
            let screenBottom = screenTop + window.innerHeight;

            let chartTop = rect.top + screenTop;
            let chartBottom = chartTop + rect.height;

            ret = !(rect.width === 0 || rect.height === 0 || chartBottom + tolerance < screenTop || chartTop - tolerance > screenBottom);
        }

        if (that.debug) {
            that.log('__isVisible(): ' + ret);
        }

        return ret;
    };

    this.isVisible = function (nocache) {
        // this.log('last_visible_check: ' + this.tm.last_visible_check + ', last_page_scroll: ' + NETDATA.options.last_page_scroll);

        // caching - we do not evaluate the charts visibility
        // if the page has not been scrolled since the last check
        if ((typeof nocache !== 'undefined' && nocache)
            || typeof this.tmp.___isVisible___ === 'undefined'
            || this.tm.last_visible_check <= NETDATA.options.last_page_scroll) {
            this.tmp.___isVisible___ = __isVisible();
            if (this.tmp.___isVisible___) {
                this.unhideChart();
            } else {
                this.hideChart();
            }
        }

        if (this.debug) {
            this.log('isVisible(' + nocache + '): ' + this.tmp.___isVisible___);
        }

        return this.tmp.___isVisible___;
    };

    this.isAutoRefreshable = function () {
        return (this.current.autorefresh);
    };

    this.canBeAutoRefreshed = function () {
        if (!this.enabled) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> not enabled');
            }

            return false;
        }

        if (this.running) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> already running');
            }

            return false;
        }

        if (this.library === null || this.library.enabled === false) {
            error('charting library "' + this.library_name + '" is not available');
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> chart library ' + this.library_name + ' is not available');
            }

            return false;
        }

        if (!this.isVisible()) {
            if (NETDATA.options.debug.visibility || this.debug) {
                this.log('canBeAutoRefreshed() -> not visible');
            }

            return false;
        }

        let now = Date.now();

        if (this.current.force_update_at !== 0 && this.current.force_update_at < now) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> timed force update - allowing this update');
            }

            this.current.force_update_at = 0;
            return true;
        }

        if (!this.isAutoRefreshable()) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> not auto-refreshable');
            }

            return false;
        }

        // allow the first update, even if the page is not visible
        if (NETDATA.options.page_is_visible === false && this.updates_counter && this.updates_since_last_unhide) {
            if (NETDATA.options.debug.focus || this.debug) {
                this.log('canBeAutoRefreshed() -> not the first update, and page does not have focus');
            }

            return false;
        }

        if (this.needsRecreation()) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> needs re-creation.');
            }

            return true;
        }

        if (NETDATA.options.auto_refresher_stop_until >= now) {
            if (this.debug) {
                this.log('canBeAutoRefreshed() -> stopped until is in future.');
            }

            return false;
        }

        // options valid only for autoRefresh()
        if (NETDATA.globalPanAndZoom.isActive()) {
            if (NETDATA.globalPanAndZoom.shouldBeAutoRefreshed(this)) {
                if (this.debug) {
                    this.log('canBeAutoRefreshed(): global panning: I need an update.');
                }

                return true;
            }
            else {
                if (this.debug) {
                    this.log('canBeAutoRefreshed(): global panning: I am already up to date.');
                }

                return false;
            }
        }

        if (this.selected) {
            if (this.debug) {
                this.log('canBeAutoRefreshed(): I have a selection in place.');
            }

            return false;
        }

        if (this.paused) {
            if (this.debug) {
                this.log('canBeAutoRefreshed(): I am paused.');
            }

            return false;
        }

        let data_update_every = this.data_update_every;
        if (typeof this.force_update_every === 'number') {
            data_update_every = this.force_update_every;
        }

        if (now - this.tm.last_autorefreshed >= data_update_every) {
            if (this.debug) {
                this.log('canBeAutoRefreshed(): It is time to update me. Now: ' + now.toString() + ', last_autorefreshed: ' + this.tm.last_autorefreshed + ', data_update_every: ' + data_update_every + ', delta: ' + (now - this.tm.last_autorefreshed).toString());
            }

            return true;
        }

        return false;
    };

    this.autoRefresh = function (callback) {
        let state = that;

        if (state.canBeAutoRefreshed() && state.running === false) {
            state.running = true;
            state.updateChart(function () {
                state.running = false;

                if (typeof callback === 'function') {
                    return callback();
                }
            });
        } else {
            if (typeof callback === 'function') {
                return callback();
            }
        }
    };

    this.__defaultsFromDownloadedChart = function (chart) {
        this.chart = chart;
        this.chart_url = chart.url;
        this.data_update_every = chart.update_every * 1000;
        this.data_points = Math.round(this.chartWidth() / this.chartPixelsPerPoint());
        this.tm.last_info_downloaded = Date.now();

        if (this.title === null) {
            this.title = chart.title;
        }

        if (this.units === null) {
            this.units = chart.units;
            this.units_current = this.units;
        }
    };

    // fetch the chart description from the netdata server
    this.getChart = function (callback) {
        this.chart = NETDATA.chartRegistry.get(this.host, this.id);
        if (this.chart) {
            this.__defaultsFromDownloadedChart(this.chart);

            if (typeof callback === 'function') {
                return callback();
            }
        } else if (netdataSnapshotData !== null) {
            // console.log(this);
            // console.log(NETDATA.chartRegistry);
            NETDATA.error(404, 'host: ' + this.host + ', chart: ' + this.id);
            error('chart not found in snapshot');

            if (typeof callback === 'function') {
                return callback();
            }
        } else {
            this.chart_url = "/api/v1/chart?chart=" + this.id;

            if (this.debug) {
                this.log('downloading ' + this.chart_url);
            }

            $.ajax({
                url: this.host + this.chart_url,
                cache: false,
                async: true,
                xhrFields: {withCredentials: true} // required for the cookie
            })
                .done(function (chart) {
                    chart = NETDATA.xss.checkOptional('/api/v1/chart', chart);

                    chart.url = that.chart_url;
                    that.__defaultsFromDownloadedChart(chart);
                    NETDATA.chartRegistry.add(that.host, that.id, chart);
                })
                .fail(function () {
                    NETDATA.error(404, that.chart_url);
                    error('chart not found on url "' + that.chart_url + '"');
                })
                .always(function () {
                    if (typeof callback === 'function') {
                        return callback();
                    }
                });
        }
    };

    // ============================================================================================================
    // INITIALIZATION

    initDOM();
    init('fast');
};

NETDATA.resetAllCharts = function (state) {
    // first clear the global selection sync
    // to make sure no chart is in selected state
    NETDATA.globalSelectionSync.stop();

    // there are 2 possibilities here
    // a. state is the global Pan and Zoom master
    // b. state is not the global Pan and Zoom master

    // let master = true;
    // if (NETDATA.globalPanAndZoom.isMaster(state) === false) {
    //     master = false;
    // }
    const master = NETDATA.globalPanAndZoom.isMaster(state)

    // clear the global Pan and Zoom
    // this will also refresh the master
    // and unblock any charts currently mirroring the master
    NETDATA.globalPanAndZoom.clearMaster();

    // if we were not the master, reset our status too
    // this is required because most probably the mouse
    // is over this chart, blocking it from auto-refreshing
    if (master === false && (state.paused || state.selected)) {
        state.resetChart();
    }
};

// get or create a chart state, given a DOM element
NETDATA.chartState = function (element) {
    let self = $(element);

    let state = self.data('netdata-state-object') || null;
    if (state === null) {
        state = new chartState(element);
        self.data('netdata-state-object', state);
    }
    return state;
};

// ----------------------------------------------------------------------------------------------------------------
// Library functions

// Load a script without jquery
// This is used to load jquery - after it is loaded, we use jquery
NETDATA._loadjQuery = function (callback) {
    if (typeof jQuery === 'undefined') {
        if (NETDATA.options.debug.main_loop) {
            console.log('loading ' + NETDATA.jQuery);
        }

        let script = document.createElement('script');
        script.type = 'text/javascript';
        script.async = true;
        script.src = NETDATA.jQuery;

        // script.onabort = onError;
        script.onerror = function () {
            NETDATA.error(101, NETDATA.jQuery);
        };
        if (typeof callback === "function") {
            script.onload = function () {
                $ = jQuery;
                return callback();
            };
        }

        let s = document.getElementsByTagName('script')[0];
        s.parentNode.insertBefore(script, s);
    }
    else if (typeof callback === "function") {
        $ = jQuery;
        return callback();
    }
};

NETDATA._loadCSS = function (filename) {
    // don't use jQuery here
    // styles are loaded before jQuery
    // to eliminate showing an unstyled page to the user

    let fileref = document.createElement("link");
    fileref.setAttribute("rel", "stylesheet");
    fileref.setAttribute("type", "text/css");
    fileref.setAttribute("href", filename);

    if (typeof fileref !== 'undefined') {
        document.getElementsByTagName("head")[0].appendChild(fileref);
    }
};

// user function to signal us the DOM has been
// updated.
NETDATA.updatedDom = function () {
    NETDATA.options.updated_dom = true;
};

NETDATA.ready = function (callback) {
    NETDATA.options.pauseCallback = callback;
};

NETDATA.pause = function (callback) {
    if (typeof callback === 'function') {
        if (NETDATA.options.pause) {
            return callback();
        } else {
            NETDATA.options.pauseCallback = callback;
        }
    }
};

NETDATA.unpause = function () {
    NETDATA.options.pauseCallback = null;
    NETDATA.options.updated_dom = true;
    NETDATA.options.pause = false;
};

// ----------------------------------------------------------------------------------------------------------------

// this is purely sequential charts refresher
// it is meant to be autonomous
NETDATA.chartRefresherNoParallel = function (index, callback) {
    let targets = NETDATA.intersectionObserver.targets();

    if (NETDATA.options.debug.main_loop) {
        console.log('NETDATA.chartRefresherNoParallel(' + index + ')');
    }

    if (NETDATA.options.updated_dom) {
        // the dom has been updated
        // get the dom parts again
        NETDATA.parseDom(callback);
        return;
    }
    if (index >= targets.length) {
        if (NETDATA.options.debug.main_loop) {
            console.log('waiting to restart main loop...');
        }

        NETDATA.options.auto_refresher_fast_weight = 0;
        callback();
    } else {
        let state = targets[index];

        if (NETDATA.options.auto_refresher_fast_weight < NETDATA.options.current.fast_render_timeframe) {
            if (NETDATA.options.debug.main_loop) {
                console.log('fast rendering...');
            }

            if (state.isVisible()) {
                NETDATA.timeout.set(function () {
                    state.autoRefresh(function () {
                        NETDATA.chartRefresherNoParallel(++index, callback);
                    });
                }, 0);
            } else {
                NETDATA.chartRefresherNoParallel(++index, callback);
            }
        } else {
            if (NETDATA.options.debug.main_loop) {
                console.log('waiting for next refresh...');
            }
            NETDATA.options.auto_refresher_fast_weight = 0;

            NETDATA.timeout.set(function () {
                state.autoRefresh(function () {
                    NETDATA.chartRefresherNoParallel(++index, callback);
                });
            }, NETDATA.options.current.idle_between_charts);
        }
    }
};

NETDATA.chartRefresherWaitTime = function () {
    return NETDATA.options.current.idle_parallel_loops;
};

// the default refresher
NETDATA.chartRefresherLastRun = 0;
NETDATA.chartRefresherRunsAfterParseDom = 0;
NETDATA.chartRefresherTimeoutId = undefined;

NETDATA.chartRefresherReschedule = function () {
    if (NETDATA.options.current.async_on_scroll) {
        if (NETDATA.chartRefresherTimeoutId) {
            NETDATA.timeout.clear(NETDATA.chartRefresherTimeoutId);
        }
        NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(NETDATA.chartRefresher, NETDATA.options.current.onscroll_worker_duration_threshold);
        //console.log('chartRefresherReschedule()');
    }
};

NETDATA.chartRefresher = function () {
    // console.log('chartRefresher() begin ' + (Date.now() - NETDATA.chartRefresherLastRun).toString() + ' ms since last run');

    if (NETDATA.options.page_is_visible === false
        && NETDATA.options.current.stop_updates_when_focus_is_lost
        && NETDATA.chartRefresherLastRun > NETDATA.options.last_page_resize
        && NETDATA.chartRefresherLastRun > NETDATA.options.last_page_scroll
        && NETDATA.chartRefresherRunsAfterParseDom > 10
    ) {
        setTimeout(
            NETDATA.chartRefresher,
            NETDATA.options.current.idle_lost_focus
        );

        // console.log('chartRefresher() page without focus, will run in ' + NETDATA.options.current.idle_lost_focus.toString() + ' ms, ' + NETDATA.chartRefresherRunsAfterParseDom.toString());
        return;
    }
    NETDATA.chartRefresherRunsAfterParseDom++;

    let now = Date.now();
    NETDATA.chartRefresherLastRun = now;

    if (now < NETDATA.options.on_scroll_refresher_stop_until) {
        NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(
            NETDATA.chartRefresher,
            NETDATA.chartRefresherWaitTime()
        );

        // console.log('chartRefresher() end1 will run in ' + NETDATA.chartRefresherWaitTime().toString() + ' ms');
        return;
    }

    if (now < NETDATA.options.auto_refresher_stop_until) {
        NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(
            NETDATA.chartRefresher,
            NETDATA.chartRefresherWaitTime()
        );

        // console.log('chartRefresher() end2 will run in ' + NETDATA.chartRefresherWaitTime().toString() + ' ms');
        return;
    }

    if (NETDATA.options.pause) {
        // console.log('auto-refresher is paused');
        NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(
            NETDATA.chartRefresher,
            NETDATA.chartRefresherWaitTime()
        );

        // console.log('chartRefresher() end3 will run in ' + NETDATA.chartRefresherWaitTime().toString() + ' ms');
        return;
    }

    if (typeof NETDATA.options.pauseCallback === 'function') {
        // console.log('auto-refresher is calling pauseCallback');

        NETDATA.options.pause = true;
        NETDATA.options.pauseCallback();
        NETDATA.chartRefresher();

        // console.log('chartRefresher() end4 (nested)');
        return;
    }

    if (!NETDATA.options.current.parallel_refresher) {
        // console.log('auto-refresher is calling chartRefresherNoParallel(0)');
        NETDATA.chartRefresherNoParallel(0, function () {
            NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(
                NETDATA.chartRefresher,
                NETDATA.options.current.idle_between_loops
            );
        });
        // console.log('chartRefresher() end5 (no parallel, nested)');
        return;
    }

    if (NETDATA.options.updated_dom) {
        // the dom has been updated
        // get the dom parts again
        // console.log('auto-refresher is calling parseDom()');
        NETDATA.parseDom(NETDATA.chartRefresher);
        // console.log('chartRefresher() end6 (parseDom)');
        return;
    }

    if (!NETDATA.globalSelectionSync.active()) {
        let parallel = [];
        let targets = NETDATA.intersectionObserver.targets();
        let len = targets.length;
        let state;
        while (len--) {
            state = targets[len];
            if (state.running || state.isVisible() === false) {
                continue;
            }

            if (!state.library.initialized) {
                if (state.library.enabled) {
                    state.library.initialize(NETDATA.chartRefresher);
                    //console.log('chartRefresher() end6 (library init)');
                    return;
                }
                else {
                    state.error('chart library "' + state.library_name + '" is not enabled.');
                }
            }

            if (NETDATA.scrollUp) {
                parallel.unshift(state);
            } else {
                parallel.push(state);
            }
        }

        len = parallel.length;
        while (len--) {
            state = parallel[len];
            // console.log('auto-refresher executing in parallel for ' + parallel.length.toString() + ' charts');
            // this will execute the jobs in parallel

            if (!state.running) {
                NETDATA.timeout.set(state.autoRefresh, 0);
            }
        }
        //else {
        //    console.log('auto-refresher nothing to do');
        //}
    }

    // run the next refresh iteration
    NETDATA.chartRefresherTimeoutId = NETDATA.timeout.set(
        NETDATA.chartRefresher,
        NETDATA.chartRefresherWaitTime()
    );

    //console.log('chartRefresher() completed in ' + (Date.now() - now).toString() + ' ms');
};

NETDATA.parseDom = function (callback) {
    //console.log('parseDom()');

    NETDATA.options.last_page_scroll = Date.now();
    NETDATA.options.updated_dom = false;
    NETDATA.chartRefresherRunsAfterParseDom = 0;

    let targets = $('div[data-netdata]'); //.filter(':visible');

    if (NETDATA.options.debug.main_loop) {
        console.log('DOM updated - there are ' + targets.length + ' charts on page.');
    }

    NETDATA.intersectionObserver.globalReset();
    NETDATA.options.targets = [];
    let len = targets.length;
    while (len--) {
        // the initialization will take care of sizing
        // and the "loading..." message
        let state = NETDATA.chartState(targets[len]);
        NETDATA.options.targets.push(state);
        NETDATA.intersectionObserver.observe(state);
    }

    if (NETDATA.globalChartUnderlay.isActive()) {
        NETDATA.globalChartUnderlay.setup();
    } else {
        NETDATA.globalChartUnderlay.clear();
    }

    if (typeof callback === 'function') {
        return callback();
    }
};

// this is the main function - where everything starts
NETDATA.started = false;
NETDATA.start = function () {
    // this should be called only once

    if (NETDATA.started) {
        console.log('netdata is already started');
        return;
    }

    NETDATA.started = true;
    NETDATA.options.page_is_visible = true;

    $(window).blur(function () {
        if (NETDATA.options.current.stop_updates_when_focus_is_lost) {
            NETDATA.options.page_is_visible = false;
            if (NETDATA.options.debug.focus) {
                console.log('Lost Focus!');
            }
        }
    });

    $(window).focus(function () {
        if (NETDATA.options.current.stop_updates_when_focus_is_lost) {
            NETDATA.options.page_is_visible = true;
            if (NETDATA.options.debug.focus) {
                console.log('Focus restored!');
            }
        }
    });

    if (typeof document.hasFocus === 'function' && !document.hasFocus()) {
        if (NETDATA.options.current.stop_updates_when_focus_is_lost) {
            NETDATA.options.page_is_visible = false;
            if (NETDATA.options.debug.focus) {
                console.log('Document has no focus!');
            }
        }
    }

    // bootstrap tab switching
    $('a[data-toggle="tab"]').on('shown.bs.tab', NETDATA.onscroll);

    // bootstrap modal switching
    let $modal = $('.modal');
    $modal.on('hidden.bs.modal', NETDATA.onscroll);
    $modal.on('shown.bs.modal', NETDATA.onscroll);

    // bootstrap collapse switching
    let $collapse = $('.collapse');
    $collapse.on('hidden.bs.collapse', NETDATA.onscroll);
    $collapse.on('shown.bs.collapse', NETDATA.onscroll);

    NETDATA.parseDom(NETDATA.chartRefresher);

    // Alarms initialization
    setTimeout(NETDATA.alarms.init, 1000);

    // Registry initialization
    setTimeout(NETDATA.registry.init, netdataRegistryAfterMs);

    if (typeof netdataCallback === 'function') {
        netdataCallback();
    }
};

NETDATA.globalReset = function () {
    NETDATA.intersectionObserver.globalReset();
    NETDATA.globalSelectionSync.globalReset();
    NETDATA.globalPanAndZoom.globalReset();
    NETDATA.chartRegistry.globalReset();
    NETDATA.commonMin.globalReset();
    NETDATA.commonMax.globalReset();
    NETDATA.commonColors.globalReset();
    NETDATA.unitsConversion.globalReset();
    NETDATA.options.targets = [];
    NETDATA.parseDom();
    NETDATA.unpause();
};
