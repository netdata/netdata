
// *** src/dashboard.js/timeout.js

// TODO: Better name needed

NETDATA.timeout = {
    // by default, these are just wrappers to setTimeout() / clearTimeout()

    step: function (callback) {
        return window.setTimeout(callback, 1000 / 60);
    },

    set: function (callback, delay) {
        return window.setTimeout(callback, delay);
    },

    clear: function (id) {
        return window.clearTimeout(id);
    },

    init: function () {
        let custom = true;

        if (window.requestAnimationFrame) {
            this.step = function (callback) {
                return window.requestAnimationFrame(callback);
            };

            this.clear = function (handle) {
                return window.cancelAnimationFrame(handle.value);
            };
        // } else if (window.webkitRequestAnimationFrame) {
        //     this.step = function (callback) {
        //         return window.webkitRequestAnimationFrame(callback);
        //     };

        //     if (window.webkitCancelAnimationFrame) {
        //         this.clear = function (handle) {
        //             return window.webkitCancelAnimationFrame(handle.value);
        //         };
        //     } else if (window.webkitCancelRequestAnimationFrame) {
        //         this.clear = function (handle) {
        //             return window.webkitCancelRequestAnimationFrame(handle.value);
        //         };
        //     }
        // } else if (window.mozRequestAnimationFrame) {
        //     this.step = function (callback) {
        //         return window.mozRequestAnimationFrame(callback);
        //     };

        //     this.clear = function (handle) {
        //         return window.mozCancelRequestAnimationFrame(handle.value);
        //     };
        // } else if (window.oRequestAnimationFrame) {
        //     this.step = function (callback) {
        //         return window.oRequestAnimationFrame(callback);
        //     };

        //     this.clear = function (handle) {
        //         return window.oCancelRequestAnimationFrame(handle.value);
        //     };
        // } else if (window.msRequestAnimationFrame) {
        //     this.step = function (callback) {
        //         return window.msRequestAnimationFrame(callback);
        //     };

        //     this.clear = function (handle) {
        //         return window.msCancelRequestAnimationFrame(handle.value);
        //     };
        } else {
            custom = false;
        }

        if (custom) {
            // we have installed custom .step() / .clear() functions
            // overwrite the .set() too

            this.set = function (callback, delay) {
                let start = Date.now(),
                    handle = new Object();

                const loop = () => {
                    let current = Date.now(),
                        delta = current - start;

                    if (delta >= delay) {
                        callback.call();
                    } else {
                        handle.value = this.step(loop);
                    }
                }

                handle.value = this.step(loop);
                return handle;
            };
        }
    }
};

NETDATA.timeout.init();
