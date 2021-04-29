// *** src/dashboard.js/compatibility.js

// Compatibility fixes.

// fix IE issue with console
if (!window.console) {
    window.console = {
        log: function () {
        }
    };
}

// if string.endsWith is not defined, define it
if (typeof String.prototype.endsWith !== 'function') {
    String.prototype.endsWith = function (s) {
        if (s.length > this.length) {
            return false;
        }
        return this.slice(-s.length) === s;
    };
}

// if string.startsWith is not defined, define it
if (typeof String.prototype.startsWith !== 'function') {
    String.prototype.startsWith = function (s) {
        if (s.length > this.length) {
            return false;
        }
        return this.slice(s.length) === s;
    };
}
