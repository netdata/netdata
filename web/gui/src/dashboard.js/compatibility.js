// ------------------------------------------------------------------------
// compatibility fixes

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

NETDATA.name2id = function (s) {
    return s
        .replace(/ /g, '_')
        .replace(/\(/g, '_')
        .replace(/\)/g, '_')
        .replace(/\./g, '_')
        .replace(/\//g, '_');
};
