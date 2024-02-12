/*!
 * d3pie
 * @author Ben Keen
 * @version 0.1.9
 * @date June 17th, 2015
 * @repo http://github.com/benkeen/d3pie
 * SPDX-License-Identifier: MIT
 */

// UMD pattern from https://github.com/umdjs/umd/blob/master/returnExports.js
(function(root, factory) {
  if (typeof define === 'function' && define.amd) {
    // AMD. Register as an anonymous module
    define([], factory);
  } else if (typeof exports === 'object') {
    // Node. Does not work with strict CommonJS, but only CommonJS-like environments that support module.exports,
    // like Node
    module.exports = factory();
  } else {
    // browser globals (root is window)
    root.d3pie = factory(root);
  }
}(this, function() {

    var _scriptName = "d3pie";
    var _version = "0.2.1";

    // used to uniquely generate IDs and classes, ensuring no conflict between multiple pies on the same page
    var _uniqueIDCounter = 0;


    // this section includes all helper libs on the d3pie object. They're populated via grunt-template. Note: to keep
    // the syntax highlighting from getting all messed up, I commented out each line. That REQUIRES each of the files
    // to have an empty first line. Crumby, yes, but acceptable.
    //// --------- _default-settings.js -----------/**
/**
 * Contains the out-the-box settings for the script. Any of these settings that aren't explicitly overridden for the
 * d3pie instance will inherit from these. This is also included on the main website for use in the generation script.
 */
var defaultSettings = {
    header: {
        title: {
            text:     "",
            color:    "#333333",
            fontSize: 18,
            fontWeight: "bold",
            font:     "arial"
        },
        subtitle: {
            text:     "",
            color:    "#666666",
            fontSize: 14,
            fontWeight: "bold",
            font:     "arial"
        },
        location: "top-center",
        titleSubtitlePadding: 8
    },
    footer: {
        text: 	  "",
        color:    "#666666",
        fontSize: 14,
        fontWeight: "bold",
        font:     "arial",
        location: "left"
    },
    size: {
        canvasHeight: 500,
        canvasWidth: 500,
        pieInnerRadius: "0%",
        pieOuterRadius: null
    },
    data: {
        sortOrder: "none",
        ignoreSmallSegments: {
            enabled: false,
            valueType: "percentage",
            value: null
        },
        smallSegmentGrouping: {
            enabled: false,
            value: 1,
            valueType: "percentage",
            label: "Other",
            color: "#cccccc"
        },
        content: []
    },
    labels: {
        outer: {
            format: "label",
            hideWhenLessThanPercentage: null,
            pieDistance: 30
        },
        inner: {
            format: "percentage",
            hideWhenLessThanPercentage: null
        },
        mainLabel: {
            color: "#333333",
            font: "arial",
            fontWeight: "normal",
            fontSize: 10
        },
        percentage: {
            color: "#dddddd",
            font: "arial",
            fontWeight: "bold",
            fontSize: 10,
            decimalPlaces: 0
        },
        value: {
            color: "#cccc44",
            fontWeight: "bold",
            font: "arial",
            fontSize: 10
        },
        lines: {
            enabled: true,
            style: "curved",
            color: "segment"
        },
        truncation: {
            enabled: false,
            truncateLength: 30
        },
    formatter: null
    },
    effects: {
        load: {
            effect: "none", // "default", commented in the code
            speed: 1000
        },
        pullOutSegmentOnClick: {
            effect: "none", // "bounce", commented in the code
            speed: 300,
            size: 10
        },
        highlightSegmentOnMouseover: false,
        highlightLuminosity: -0.2
    },
    tooltips: {
        enabled: false,
        type: "placeholder", // caption|placeholder
        string: "",
        placeholderParser: null,
        styles: {
        fadeInSpeed: 250,
        backgroundColor: "#000000",
        backgroundOpacity: 0.5,
        color: "#efefef",
        borderRadius: 2,
        font: "arial",
        fontWeight: "bold",
        fontSize: 10,
        padding: 4
        }
    },
    misc: {
        colors: {
            background: null,
            segments: [
                "#2484c1", "#65a620", "#7b6888", "#a05d56", "#961a1a", "#d8d23a", "#e98125", "#d0743c", "#635222", "#6ada6a",
                "#0c6197", "#7d9058", "#207f33", "#44b9b0", "#bca44a", "#e4a14b", "#a3acb2", "#8cc3e9", "#69a6f9", "#5b388f",
                "#546e91", "#8bde95", "#d2ab58", "#273c71", "#98bf6e", "#4daa4b", "#98abc5", "#cc1010", "#31383b", "#006391",
                "#c2643f", "#b0a474", "#a5a39c", "#a9c2bc", "#22af8c", "#7fcecf", "#987ac6", "#3d3b87", "#b77b1c", "#c9c2b6",
                "#807ece", "#8db27c", "#be66a2", "#9ed3c6", "#00644b", "#005064", "#77979f", "#77e079", "#9c73ab", "#1f79a7"
            ],
            segmentStroke: "#ffffff"
        },
        gradient: {
            enabled: false,
            percentage: 95,
            color: "#000000"
        },
        canvasPadding: {
            top: 5,
            right: 5,
            bottom: 5,
            left: 5
        },
        pieCenterOffset: {
            x: 0,
            y: 0
        },
        cssPrefix: null
    },
    callbacks: {
        onload: null,
        onMouseoverSegment: null,
        onMouseoutSegment: null,
        onClickSegment: null
    }
};

    //// --------- validate.js -----------
var validate = {

    // called whenever a new pie chart is created
    initialCheck: function(pie) {
        var cssPrefix = pie.cssPrefix;
        var element = pie.element;
        var options = pie.options;

        // confirm d3 is available [check minimum version]
        if (!window.d3 || !window.d3.hasOwnProperty("version")) {
            console.error("d3pie error: d3 is not available");
            return false;
        }

        // confirm element is either a DOM element or a valid string for a DOM element
        if (!(element instanceof HTMLElement || element instanceof SVGElement)) {
            console.error("d3pie error: the first d3pie() param must be a valid DOM element (not jQuery) or a ID string.");
            return false;
        }

        // confirm the CSS prefix is valid. It has to start with a-Z and contain nothing but a-Z0-9_-
        if (!(/[a-zA-Z][a-zA-Z0-9_-]*$/.test(cssPrefix))) {
            console.error("d3pie error: invalid options.misc.cssPrefix");
            return false;
        }

        // confirm some data has been supplied
        if (!helpers.isArray(options.data.content)) {
            console.error("d3pie error: invalid config structure: missing data.content property.");
            return false;
        }
        if (options.data.content.length === 0) {
            console.error("d3pie error: no data supplied.");
            return false;
        }

        // clear out any invalid data. Each data row needs a valid positive number and a label
        var data = [];
        for (var i=0; i<options.data.content.length; i++) {
            if (typeof options.data.content[i].value !== "number" || isNaN(options.data.content[i].value)) {
                console.log("not valid: ", options.data.content[i]);
                continue;
            }
            if (options.data.content[i].value <= 0) {
                console.log("not valid - should have positive value: ", options.data.content[i]);
                continue;
            }
            data.push(options.data.content[i]);
        }
        pie.options.data.content = data;

        // labels.outer.hideWhenLessThanPercentage - 1-100
        // labels.inner.hideWhenLessThanPercentage - 1-100

        return true;
    }
};

    //// --------- helpers.js -----------
var helpers = {

    // creates the SVG element
    addSVGSpace: function(pie) {
        var element = pie.element;
        var canvasWidth = pie.options.size.canvasWidth;
        var canvasHeight = pie.options.size.canvasHeight;
        var backgroundColor = pie.options.misc.colors.background;

        var svg = d3.select(element).append("svg:svg")
            .attr("width", canvasWidth)
            .attr("height", canvasHeight);

        if (backgroundColor !== "transparent") {
            svg.style("background-color", function() { return backgroundColor; });
        }

        return svg;
    },

    shuffleArray: function(array) {
        var currentIndex = array.length, tmpVal, randomIndex;

        while (0 !== currentIndex) {
            randomIndex = Math.floor(Math.random() * currentIndex);
            currentIndex -= 1;

            // and swap it with the current element
            tmpVal = array[currentIndex];
            array[currentIndex] = array[randomIndex];
            array[randomIndex] = tmpVal;
        }
        return array;
    },

    processObj: function(obj, is, value) {
        if (typeof is === 'string') {
            return helpers.processObj(obj, is.split('.'), value);
        } else if (is.length === 1 && value !== undefined) {
            obj[is[0]] = value;
            return obj[is[0]];
        } else if (is.length === 0) {
            return obj;
        } else {
            return helpers.processObj(obj[is[0]], is.slice(1), value);
        }
    },

    getDimensions: function(el) {
        if(typeof el === 'string')
            el = document.getElementById(el);

        var w = 0, h = 0;
        if (el) {
            var dimensions = el.getBBox();
            w = dimensions.width;
            h = dimensions.height;
        }
        else {
            console.log("error: getDimensions() " + id + " not found.");
        }

        return { w: w, h: h };
    },

    /**
     * This is based on the SVG coordinate system, where top-left is 0,0 and bottom right is n-n.
     * @param r1
     * @param r2
     * @returns {boolean}
     */
    rectIntersect: function(r1, r2) {
        var returnVal = (
            // r2.left > r1.right
            (r2.x > (r1.x + r1.w)) ||

            // r2.right < r1.left
            ((r2.x + r2.w) < r1.x) ||

            // r2.top < r1.bottom
            ((r2.y + r2.h) < r1.y) ||

            // r2.bottom > r1.top
            (r2.y > (r1.y + r1.h))
        );

        return !returnVal;
    },

    /**
     * Returns a lighter/darker shade of a hex value, based on a luminance value passed.
     * @param hex a hex color value such as “#abc” or “#123456″ (the hash is optional)
     * @param lum the luminosity factor: -0.1 is 10% darker, 0.2 is 20% lighter, etc.
     * @returns {string}
     */
    getColorShade: function(hex, lum) {

        // validate hex string
        hex = String(hex).replace(/[^0-9a-f]/gi, '');
        if (hex.length < 6) {
            hex = hex[0]+hex[0]+hex[1]+hex[1]+hex[2]+hex[2];
        }
        lum = lum || 0;

        // convert to decimal and change luminosity
        var newHex = "#";
        for (var i=0; i<3; i++) {
            var c = parseInt(hex.substr(i * 2, 2), 16);
            c = Math.round(Math.min(Math.max(0, c + (c * lum)), 255)).toString(16);
            newHex += ("00" + c).substr(c.length);
        }

        return newHex;
    },

    /**
     * Users can choose to specify segment colors in three ways (in order of precedence):
     * 	1. include a "color" attribute for each row in data.content
     * 	2. include a misc.colors.segments property which contains an array of hex codes
     * 	3. specify nothing at all and rely on this lib provide some reasonable defaults
     *
     * This function sees what's included and populates this.options.colors with whatever's required
     * for this pie chart.
     * @param data
     */
    initSegmentColors: function(pie) {
        var data   = pie.options.data.content;
        var colors = pie.options.misc.colors.segments;

        // TODO this needs a ton of error handling

        var finalColors = [];
        for (var i=0; i<data.length; i++) {
            if (data[i].hasOwnProperty("color")) {
                finalColors.push(data[i].color);
            } else {
                finalColors.push(colors[i]);
            }
        }

        return finalColors;
    },

    applySmallSegmentGrouping: function(data, smallSegmentGrouping) {
        var totalSize;
        if (smallSegmentGrouping.valueType === "percentage") {
            totalSize = math.getTotalPieSize(data);
        }

        // loop through each data item
        var newData = [];
        var groupedData = [];
        var totalGroupedData = 0;
        for (var i=0; i<data.length; i++) {
            if (smallSegmentGrouping.valueType === "percentage") {
                var dataPercent = (data[i].value / totalSize) * 100;
                if (dataPercent <= smallSegmentGrouping.value) {
                    groupedData.push(data[i]);
                    totalGroupedData += data[i].value;
                    continue;
                }
                data[i].isGrouped = false;
                newData.push(data[i]);
            } else {
                if (data[i].value <= smallSegmentGrouping.value) {
                    groupedData.push(data[i]);
                    totalGroupedData += data[i].value;
                    continue;
                }
                data[i].isGrouped = false;
                newData.push(data[i]);
            }
        }

        // we're done! See if there's any small segment groups to add
        if (groupedData.length) {
            newData.push({
                color: smallSegmentGrouping.color,
                label: smallSegmentGrouping.label,
                value: totalGroupedData,
                isGrouped: true,
                groupedData: groupedData
            });
        }

        return newData;
    },

    // for debugging
    showPoint: function(svg, x, y) {
        svg.append("circle").attr("cx", x).attr("cy", y).attr("r", 2).style("fill", "black");
    },

    isFunction: function(functionToCheck) {
        var getType = {};
        return functionToCheck && getType.toString.call(functionToCheck) === '[object Function]';
    },

    isArray: function(o) {
        return Object.prototype.toString.call(o) === '[object Array]';
    }
};


// taken from jQuery
var extend = function() {
    var options, name, src, copy, copyIsArray, clone, target = arguments[0] || {},
        i = 1,
        length = arguments.length,
        deep = false,
        toString = Object.prototype.toString,
        hasOwn = Object.prototype.hasOwnProperty,
        class2type = {
            "[object Boolean]": "boolean",
            "[object Number]": "number",
            "[object String]": "string",
            "[object Function]": "function",
            "[object Array]": "array",
            "[object Date]": "date",
            "[object RegExp]": "regexp",
            "[object Object]": "object"
        },

        jQuery = {
            isFunction: function (obj) {
                return jQuery.type(obj) === "function";
            },
            isArray: Array.isArray ||
                function (obj) {
                    return jQuery.type(obj) === "array";
                },
            isWindow: function (obj) {
                return obj !== null && obj === obj.window;
            },
            isNumeric: function (obj) {
                return !isNaN(parseFloat(obj)) && isFinite(obj);
            },
            type: function (obj) {
                return obj === null ? String(obj) : class2type[toString.call(obj)] || "object";
            },
            isPlainObject: function (obj) {
                if (!obj || jQuery.type(obj) !== "object" || obj.nodeType) {
                    return false;
                }
                try {
                    if (obj.constructor && !hasOwn.call(obj, "constructor") && !hasOwn.call(obj.constructor.prototype, "isPrototypeOf")) {
                        return false;
                    }
                } catch (e) {
                    return false;
                }
                var key;
                for (key in obj) {}
                return key === undefined || hasOwn.call(obj, key);
            }
        };
    if (typeof target === "boolean") {
        deep = target;
        target = arguments[1] || {};
        i = 2;
    }
    if (typeof target !== "object" && !jQuery.isFunction(target)) {
        target = {};
    }
    if (length === i) {
        target = this;
        --i;
    }
    for (i; i < length; i++) {
        if ((options = arguments[i]) !== null) {
            for (name in options) {
                src = target[name];
                copy = options[name];
                if (target === copy) {
                    continue;
                }
                if (deep && copy && (jQuery.isPlainObject(copy) || (copyIsArray = jQuery.isArray(copy)))) {
                    if (copyIsArray) {
                        copyIsArray = false;
                        clone = src && jQuery.isArray(src) ? src : [];
                    } else {
                        clone = src && jQuery.isPlainObject(src) ? src : {};
                    }
                    // WARNING: RECURSION
                    target[name] = extend(deep, clone, copy);
                } else if (copy !== undefined) {
                    target[name] = copy;
                }
            }
        }
    }
    return target;
};
    //// --------- math.js -----------
var math = {

    toRadians: function(degrees) {
        return degrees * (Math.PI / 180);
    },

    toDegrees: function(radians) {
        return radians * (180 / Math.PI);
    },

    computePieRadius: function(pie) {
        var size = pie.options.size;
        var canvasPadding = pie.options.misc.canvasPadding;

        // outer radius is either specified (e.g. through the generator), or omitted altogether
        // and calculated based on the canvas dimensions. Right now the estimated version isn't great - it should
        // be possible to calculate it to precisely generate the maximum sized pie, but it's fussy as heck. Something
        // for the next release.

        // first, calculate the default _outerRadius
        var w = size.canvasWidth - canvasPadding.left - canvasPadding.right;
        var h = size.canvasHeight - canvasPadding.top - canvasPadding.bottom;

        // now factor in the footer, title & subtitle
        if (pie.options.header.location !== "pie-center") {
            h -= pie.textComponents.headerHeight;
        }

        if (pie.textComponents.footer.exists) {
            h -= pie.textComponents.footer.h;
        }

        // for really teeny pies, h may be < 0. Adjust it back
        h = (h < 0) ? 0 : h;

        var outerRadius = ((w < h) ? w : h) / 3;
        var innerRadius, percent;

        // if the user specified something, use that instead
        if (size.pieOuterRadius !== null) {
            if (/%/.test(size.pieOuterRadius)) {
                percent = parseInt(size.pieOuterRadius.replace(/[\D]/, ""), 10);
                percent = (percent > 99) ? 99 : percent;
                percent = (percent < 0) ? 0 : percent;

                var smallestDimension = (w < h) ? w : h;

                // now factor in the label line size
                if (pie.options.labels.outer.format !== "none") {
                    var pieDistanceSpace = parseInt(pie.options.labels.outer.pieDistance, 10) * 2;
                    if (smallestDimension - pieDistanceSpace > 0) {
                        smallestDimension -= pieDistanceSpace;
                    }
                }

                outerRadius = Math.floor((smallestDimension / 100) * percent) / 2;
            } else {
                outerRadius = parseInt(size.pieOuterRadius, 10);
            }
        }

        // inner radius
        if (/%/.test(size.pieInnerRadius)) {
            percent = parseInt(size.pieInnerRadius.replace(/[\D]/, ""), 10);
            percent = (percent > 99) ? 99 : percent;
            percent = (percent < 0) ? 0 : percent;
            innerRadius = Math.floor((outerRadius / 100) * percent);
        } else {
            innerRadius = parseInt(size.pieInnerRadius, 10);
        }

        pie.innerRadius = innerRadius;
        pie.outerRadius = outerRadius;
    },

    getTotalPieSize: function(data) {
        var totalSize = 0;
        for (var i=0; i<data.length; i++) {
            totalSize += data[i].value;
        }
        return totalSize;
    },

    sortPieData: function(pie) {
        var data                 = pie.options.data.content;
        var sortOrder            = pie.options.data.sortOrder;

        switch (sortOrder) {
            case "none":
                // do nothing
                break;
            case "random":
                data = helpers.shuffleArray(data);
                break;
            case "value-asc":
                data.sort(function(a, b) { return (a.value < b.value) ? -1 : 1; });
                break;
            case "value-desc":
                data.sort(function(a, b) { return (a.value < b.value) ? 1 : -1; });
                break;
            case "label-asc":
                data.sort(function(a, b) { return (a.label.toLowerCase() > b.label.toLowerCase()) ? 1 : -1; });
                break;
            case "label-desc":
                data.sort(function(a, b) { return (a.label.toLowerCase() < b.label.toLowerCase()) ? 1 : -1; });
                break;
        }

        return data;
    },

    // var pieCenter = math.getPieCenter();
    getPieTranslateCenter: function(pieCenter) {
        return "translate(" + pieCenter.x + "," + pieCenter.y + ")";
    },

    /**
     * Used to determine where on the canvas the center of the pie chart should be. It takes into account the
     * height and position of the title, subtitle and footer, and the various paddings.
     * @private
     */
    calculatePieCenter: function(pie) {
        var pieCenterOffset = pie.options.misc.pieCenterOffset;
        var hasTopTitle    = (pie.textComponents.title.exists && pie.options.header.location !== "pie-center");
        var hasTopSubtitle = (pie.textComponents.subtitle.exists && pie.options.header.location !== "pie-center");

        var headerOffset = pie.options.misc.canvasPadding.top;
        if (hasTopTitle && hasTopSubtitle) {
            headerOffset += pie.textComponents.title.h + pie.options.header.titleSubtitlePadding + pie.textComponents.subtitle.h;
        } else if (hasTopTitle) {
            headerOffset += pie.textComponents.title.h;
        } else if (hasTopSubtitle) {
            headerOffset += pie.textComponents.subtitle.h;
        }

        var footerOffset = 0;
        if (pie.textComponents.footer.exists) {
            footerOffset = pie.textComponents.footer.h + pie.options.misc.canvasPadding.bottom;
        }

        var x = ((pie.options.size.canvasWidth - pie.options.misc.canvasPadding.left - pie.options.misc.canvasPadding.right) / 2) + pie.options.misc.canvasPadding.left;
        var y = ((pie.options.size.canvasHeight - footerOffset - headerOffset) / 2) + headerOffset;

        x += pieCenterOffset.x;
        y += pieCenterOffset.y;

        pie.pieCenter = { x: x, y: y };
    },


    /**
     * Rotates a point (x, y) around an axis (xm, ym) by degrees (a).
     * @param x
     * @param y
     * @param xm
     * @param ym
     * @param a angle in degrees
     * @returns {Array}
     */
    rotate: function(x, y, xm, ym, a) {

        a = a * Math.PI / 180; // convert to radians

        var cos = Math.cos,
            sin = Math.sin,
        // subtract midpoints, so that midpoint is translated to origin and add it in the end again
        xr = (x - xm) * cos(a) - (y - ym) * sin(a) + xm,
        yr = (x - xm) * sin(a) + (y - ym) * cos(a) + ym;

        return { x: xr, y: yr };
    },

    /**
     * Translates a point x, y by distance d, and by angle a.
     * @param x
     * @param y
     * @param dist
     * @param a angle in degrees
     */
    translate: function(x, y, d, a) {
        var rads = math.toRadians(a);
        return {
            x: x + d * Math.sin(rads),
            y: y - d * Math.cos(rads)
        };
    },

    // from: http://stackoverflow.com/questions/19792552/d3-put-arc-labels-in-a-pie-chart-if-there-is-enough-space
    pointIsInArc: function(pt, ptData, d3Arc) {
        // Center of the arc is assumed to be 0,0
        // (pt.x, pt.y) are assumed to be relative to the center
        var r1 = d3Arc.innerRadius()(ptData), // Note: Using the innerRadius
            r2 = d3Arc.outerRadius()(ptData),
            theta1 = d3Arc.startAngle()(ptData),
            theta2 = d3Arc.endAngle()(ptData);

        var dist = pt.x * pt.x + pt.y * pt.y,
            angle = Math.atan2(pt.x, -pt.y); // Note: different coordinate system

        angle = (angle < 0) ? (angle + Math.PI * 2) : angle;

        return (r1 * r1 <= dist) && (dist <= r2 * r2) &&
            (theta1 <= angle) && (angle <= theta2);
    }
};

    //// --------- labels.js -----------
var labels = {

    /**
     * Adds the labels to the pie chart, but doesn't position them. There are two locations for the
     * labels: inside (center) of the segments, or outside the segments on the edge.
     * @param section "inner" or "outer"
     * @param sectionDisplayType "percentage", "value", "label", "label-value1", etc.
     * @param pie
     */
    add: function(pie, section, sectionDisplayType) {
        var include = labels.getIncludes(sectionDisplayType);
        var settings = pie.options.labels;

        // group the label groups (label, percentage, value) into a single element for simpler positioning
        var outerLabel = pie.svg.insert("g", "." + pie.cssPrefix + "labels-" + section)
            .attr("class", pie.cssPrefix + "labels-" + section);

        var labelGroup = pie.__labels[section] = outerLabel.selectAll("." + pie.cssPrefix + "labelGroup-" + section)
            .data(pie.options.data.content)
            .enter()
            .append("g")
            .attr("id", function(d, i) { return pie.cssPrefix + "labelGroup" + i + "-" + section; })
            .attr("data-index", function(d, i) { return i; })
            .attr("class", pie.cssPrefix + "labelGroup-" + section)
            .style("opacity", 0);

        var formatterContext = { section: section, sectionDisplayType: sectionDisplayType };

        // 1. Add the main label
        if (include.mainLabel) {
            labelGroup.append("text")
                .attr("id", function(d, i) { return pie.cssPrefix + "segmentMainLabel" + i + "-" + section; })
                .attr("class", pie.cssPrefix + "segmentMainLabel-" + section)
                .text(function(d, i) {
                    var str = d.label;

                    // if a custom formatter has been defined, pass it the raw label string - it can do whatever it wants with it.
                    // we only apply truncation if it's not defined
                    if (settings.formatter) {
                        formatterContext.index = i;
                        formatterContext.part = 'mainLabel';
                        formatterContext.value = d.value;
                        formatterContext.label = str;
                        str = settings.formatter(formatterContext);
                    } else if (settings.truncation.enabled && d.label.length > settings.truncation.truncateLength) {
                        str = d.label.substring(0, settings.truncation.truncateLength) + "...";
                    }
                    return str;
                })
                .style("font-size", settings.mainLabel.fontSize + "px")
                .style("font-family", settings.mainLabel.font)
                .style("font-weight", settings.mainLabel.fontWeight)
                .style("fill", function(d, i) {
                    return (settings.mainLabel.color === "segment") ? pie.options.colors[i] : settings.mainLabel.color;
                });
        }

        // 2. Add the percentage label
        if (include.percentage) {
            labelGroup.append("text")
                .attr("id", function(d, i) { return pie.cssPrefix + "segmentPercentage" + i + "-" + section; })
                .attr("class", pie.cssPrefix + "segmentPercentage-" + section)
                .text(function(d, i) {
                    var percentage = d.percentage;
                    if (settings.formatter) {
                        formatterContext.index = i;
                        formatterContext.part = "percentage";
                        formatterContext.value = d.value;
                        formatterContext.label = d.percentage;
                        percentage = settings.formatter(formatterContext);
                    } else {
                        percentage += "%";
                    }
                    return percentage;
                })
                .style("font-size", settings.percentage.fontSize + "px")
                .style("font-family", settings.percentage.font)
                .style("font-weight", settings.percentage.fontWeight)
                .style("fill", settings.percentage.color);
        }

        // 3. Add the value label
        if (include.value) {
            labelGroup.append("text")
                .attr("id", function(d, i) { return pie.cssPrefix +  "segmentValue" + i + "-" + section; })
                .attr("class", pie.cssPrefix + "segmentValue-" + section)
                .text(function(d, i) {
                    formatterContext.index = i;
                    formatterContext.part = "value";
                    formatterContext.value = d.value;
                    formatterContext.label = d.value;
                    return settings.formatter ? settings.formatter(formatterContext, d.value) : d.value;
                })
                .style("font-size", settings.value.fontSize + "px")
                .style("font-family", settings.value.font)
                .style("font-weight", settings.value.fontWeight)
                .style("fill", settings.value.color);
        }
    },

    /**
     * @param section "inner" / "outer"
     */
    positionLabelElements: function(pie, section, sectionDisplayType) {
        labels["dimensions-" + section] = [];

        // get the latest widths, heights
        var labelGroups = pie.__labels[section];
        labelGroups.each(function(d, i) {
            var mainLabel  = d3.select(this).selectAll("." + pie.cssPrefix + "segmentMainLabel-" + section);
            var percentage = d3.select(this).selectAll("." + pie.cssPrefix + "segmentPercentage-" + section);
            var value      = d3.select(this).selectAll("." + pie.cssPrefix + "segmentValue-" + section);

            labels["dimensions-" + section].push({
                mainLabel:  (mainLabel.node() !== null) ? mainLabel.node().getBBox() : null,
                percentage: (percentage.node() !== null) ? percentage.node().getBBox() : null,
                value:      (value.node() !== null) ? value.node().getBBox() : null
            });
        });

        var singleLinePad = 5;
        var dims = labels["dimensions-" + section];
        switch (sectionDisplayType) {
            case "label-value1":
                pie.svg.selectAll("." + pie.cssPrefix + "segmentValue-" + section)
                    .attr("dx", function(d, i) { return dims[i].mainLabel.width + singleLinePad; });
                break;
            case "label-value2":
                pie.svg.selectAll("." + pie.cssPrefix + "segmentValue-" + section)
                    .attr("dy", function(d, i) { return dims[i].mainLabel.height; });
                break;
            case "label-percentage1":
                pie.svg.selectAll("." + pie.cssPrefix + "segmentPercentage-" + section)
                    .attr("dx", function(d, i) { return dims[i].mainLabel.width + singleLinePad; });
                break;
            case "label-percentage2":
                pie.svg.selectAll("." + pie.cssPrefix + "segmentPercentage-" + section)
                    .attr("dx", function(d, i) { return (dims[i].mainLabel.width / 2) - (dims[i].percentage.width / 2); })
                    .attr("dy", function(d, i) { return dims[i].mainLabel.height; });
                break;
        }
    },

    computeLabelLinePositions: function(pie) {
        pie.lineCoordGroups = [];
        pie.__labels.outer
            .each(function(d, i) { return labels.computeLinePosition(pie, i); });
    },

    computeLinePosition: function(pie, i) {
        var angle = segments.getSegmentAngle(i, pie.options.data.content, pie.totalSize, { midpoint: true });
        var originCoords = math.rotate(pie.pieCenter.x, pie.pieCenter.y - pie.outerRadius, pie.pieCenter.x, pie.pieCenter.y, angle);
        var heightOffset = pie.outerLabelGroupData[i].h / 5; // TODO check
        var labelXMargin = 6; // the x-distance of the label from the end of the line [TODO configurable]

        var quarter = Math.floor(angle / 90);
        var midPoint = 4;
        var x2, y2, x3, y3;

        // this resolves an issue when the
        if (quarter === 2 && angle === 180) {
            quarter = 1;
        }

        switch (quarter) {
            case 0:
                x2 = pie.outerLabelGroupData[i].x - labelXMargin - ((pie.outerLabelGroupData[i].x - labelXMargin - originCoords.x) / 2);
                y2 = pie.outerLabelGroupData[i].y + ((originCoords.y - pie.outerLabelGroupData[i].y) / midPoint);
                x3 = pie.outerLabelGroupData[i].x - labelXMargin;
                y3 = pie.outerLabelGroupData[i].y - heightOffset;
                break;
            case 1:
                x2 = originCoords.x + (pie.outerLabelGroupData[i].x - originCoords.x) / midPoint;
                y2 = originCoords.y + (pie.outerLabelGroupData[i].y - originCoords.y) / midPoint;
                x3 = pie.outerLabelGroupData[i].x - labelXMargin;
                y3 = pie.outerLabelGroupData[i].y - heightOffset;
                break;
            case 2:
                var startOfLabelX = pie.outerLabelGroupData[i].x + pie.outerLabelGroupData[i].w + labelXMargin;
                x2 = originCoords.x - (originCoords.x - startOfLabelX) / midPoint;
                y2 = originCoords.y + (pie.outerLabelGroupData[i].y - originCoords.y) / midPoint;
                x3 = pie.outerLabelGroupData[i].x + pie.outerLabelGroupData[i].w + labelXMargin;
                y3 = pie.outerLabelGroupData[i].y - heightOffset;
                break;
            case 3:
                var startOfLabel = pie.outerLabelGroupData[i].x + pie.outerLabelGroupData[i].w + labelXMargin;
                x2 = startOfLabel + ((originCoords.x - startOfLabel) / midPoint);
                y2 = pie.outerLabelGroupData[i].y + (originCoords.y - pie.outerLabelGroupData[i].y) / midPoint;
                x3 = pie.outerLabelGroupData[i].x + pie.outerLabelGroupData[i].w + labelXMargin;
                y3 = pie.outerLabelGroupData[i].y - heightOffset;
                break;
        }

        /*
         * x1 / y1: the x/y coords of the start of the line, at the mid point of the segments arc on the pie circumference
         * x2 / y2: if "curved" line style is being used, this is the midpoint of the line. Other
         * x3 / y3: the end of the line; closest point to the label
         */
        if (pie.options.labels.lines.style === "straight") {
            pie.lineCoordGroups[i] = [
                { x: originCoords.x, y: originCoords.y },
                { x: x3, y: y3 }
            ];
        } else {
            pie.lineCoordGroups[i] = [
                { x: originCoords.x, y: originCoords.y },
                { x: x2, y: y2 },
                { x: x3, y: y3 }
            ];
        }
    },

    addLabelLines: function(pie) {
        var lineGroups = pie.svg.insert("g", "." + pie.cssPrefix + "pieChart") // meaning, BEFORE .pieChart
            .attr("class", pie.cssPrefix + "lineGroups")
            .style("opacity", 1);

        var lineGroup = lineGroups.selectAll("." + pie.cssPrefix + "lineGroup")
            .data(pie.lineCoordGroups)
            .enter()
            .append("g")
            .attr("class", pie.cssPrefix + "lineGroup");

        var lineFunction = d3.line()
            .curve(d3.curveBasis)
            .x(function(d) { return d.x; })
            .y(function(d) { return d.y; });

        lineGroup.append("path")
            .attr("d", lineFunction)
            .attr("stroke", function(d, i) {
                return (pie.options.labels.lines.color === "segment") ? pie.options.colors[i] : pie.options.labels.lines.color;
            })
            .attr("stroke-width", 1)
            .attr("fill", "none")
            .style("opacity", function(d, i) {
                var percentage = pie.options.labels.outer.hideWhenLessThanPercentage;
                var isHidden = (percentage !== null && d.percentage < percentage) || pie.options.data.content[i].label === "";
                return isHidden ? 0 : 1;
            });
    },

    positionLabelGroups: function(pie, section) {
        if (pie.options.labels[section].format === "none")
            return;

        pie.__labels[section]
            .style("opacity", function(d, i) {
                var percentage = pie.options.labels[section].hideWhenLessThanPercentage;
                return (percentage !== null && d.percentage < percentage) ? 0 : 1;
            })
            .attr("transform", function(d, i) {
                var x, y;
                if (section === "outer") {
                    x = pie.outerLabelGroupData[i].x;
                    y = pie.outerLabelGroupData[i].y;
                } else {
                    var pieCenterCopy = extend(true, {}, pie.pieCenter);

                    // now recompute the "center" based on the current _innerRadius
                    if (pie.innerRadius > 0) {
                        var angle = segments.getSegmentAngle(i, pie.options.data.content, pie.totalSize, { midpoint: true });
                        var newCoords = math.translate(pie.pieCenter.x, pie.pieCenter.y, pie.innerRadius, angle);
                        pieCenterCopy.x = newCoords.x;
                        pieCenterCopy.y = newCoords.y;
                    }

                    var dims = helpers.getDimensions(pie.cssPrefix + "labelGroup" + i + "-inner");
                    var xOffset = dims.w / 2;
                    var yOffset = dims.h / 4; // confusing! Why 4? should be 2, but it doesn't look right

                    x = pieCenterCopy.x + (pie.lineCoordGroups[i][0].x - pieCenterCopy.x) / 1.8;
                    y = pieCenterCopy.y + (pie.lineCoordGroups[i][0].y - pieCenterCopy.y) / 1.8;

                    x = x - xOffset;
                    y = y + yOffset;
                }

                return "translate(" + x + "," + y + ")";
            });
    },


    getIncludes: function(val) {
        var addMainLabel  = false;
        var addValue      = false;
        var addPercentage = false;

        switch (val) {
            case "label":
                addMainLabel = true;
                break;
            case "value":
                addValue = true;
                break;
            case "percentage":
                addPercentage = true;
                break;
            case "label-value1":
            case "label-value2":
                addMainLabel = true;
                addValue = true;
                break;
            case "label-percentage1":
            case "label-percentage2":
                addMainLabel = true;
                addPercentage = true;
                break;
        }
        return {
            mainLabel: addMainLabel,
            value: addValue,
            percentage: addPercentage
        };
    },


    /**
     * This does the heavy-lifting to compute the actual coordinates for the outer label groups. It does two things:
     * 1. Make a first pass and position them in the ideal positions, based on the pie sizes
     * 2. Do some basic collision avoidance.
     */
    computeOuterLabelCoords: function(pie) {

        // 1. figure out the ideal positions for the outer labels
        pie.__labels.outer
            .each(function(d, i) {
                return labels.getIdealOuterLabelPositions(pie, i);
            });

        // 2. now adjust those positions to try to accommodate conflicts
        labels.resolveOuterLabelCollisions(pie);
    },

    /**
     * This attempts to resolve label positioning collisions.
     */
    resolveOuterLabelCollisions: function(pie) {
    if (pie.options.labels.outer.format === "none") {
      return;
    }

        var size = pie.options.data.content.length;
        labels.checkConflict(pie, 0, "clockwise", size);
        labels.checkConflict(pie, size-1, "anticlockwise", size);
    },

    checkConflict: function(pie, currIndex, direction, size) {
    var i, curr;

        if (size <= 1) {
            return;
        }

        var currIndexHemisphere = pie.outerLabelGroupData[currIndex].hs;
        if (direction === "clockwise" && currIndexHemisphere !== "right") {
            return;
        }
        if (direction === "anticlockwise" && currIndexHemisphere !== "left") {
            return;
        }
        var nextIndex = (direction === "clockwise") ? currIndex+1 : currIndex-1;

        // this is the current label group being looked at. We KNOW it's positioned properly (the first item
        // is always correct)
        var currLabelGroup = pie.outerLabelGroupData[currIndex];

        // this one we don't know about. That's the one we're going to look at and move if necessary
        var examinedLabelGroup = pie.outerLabelGroupData[nextIndex];

        var info = {
            labelHeights: pie.outerLabelGroupData[0].h,
            center: pie.pieCenter,
            lineLength: (pie.outerRadius + pie.options.labels.outer.pieDistance),
            heightChange: pie.outerLabelGroupData[0].h + 1 // 1 = padding
        };

        // loop through *ALL* label groups examined so far to check for conflicts. This is because when they're
        // very tightly fitted, a later label group may still appear high up on the page
        if (direction === "clockwise") {
      i = 0;
            for (; i<=currIndex; i++) {
                curr = pie.outerLabelGroupData[i];

                // if there's a conflict with this label group, shift the label to be AFTER the last known
                // one that's been properly placed
                if (!labels.isLabelHidden(pie, i) && helpers.rectIntersect(curr, examinedLabelGroup)) {
                    labels.adjustLabelPos(pie, nextIndex, currLabelGroup, info);
                    break;
                }
            }
        } else {
      i = size - 1;
            for (; i >= currIndex; i--) {
                curr = pie.outerLabelGroupData[i];

                // if there's a conflict with this label group, shift the label to be AFTER the last known
                // one that's been properly placed
                if (!labels.isLabelHidden(pie, i) && helpers.rectIntersect(curr, examinedLabelGroup)) {
                    labels.adjustLabelPos(pie, nextIndex, currLabelGroup, info);
                    break;
                }
            }
        }
        labels.checkConflict(pie, nextIndex, direction, size);
    },

    isLabelHidden: function(pie, index) {
        var percentage = pie.options.labels.outer.hideWhenLessThanPercentage;
        return (percentage !== null && d.percentage < percentage) || pie.options.data.content[index].label === "";
    },

    // does a little math to shift a label into a new position based on the last properly placed one
    adjustLabelPos: function(pie, nextIndex, lastCorrectlyPositionedLabel, info) {
        var xDiff, yDiff, newXPos, newYPos;
        newYPos = lastCorrectlyPositionedLabel.y + info.heightChange;
        yDiff = info.center.y - newYPos;

        if (Math.abs(info.lineLength) > Math.abs(yDiff)) {
            xDiff = Math.sqrt((info.lineLength * info.lineLength) - (yDiff * yDiff));
        } else {
            xDiff = Math.sqrt((yDiff * yDiff) - (info.lineLength * info.lineLength));
        }

        if (lastCorrectlyPositionedLabel.hs === "right") {
            newXPos = info.center.x + xDiff;
        } else {
            newXPos = info.center.x - xDiff - pie.outerLabelGroupData[nextIndex].w;
        }

        pie.outerLabelGroupData[nextIndex].x = newXPos;
        pie.outerLabelGroupData[nextIndex].y = newYPos;
    },

    /**
     * @param i 0-N where N is the dataset size - 1.
     */
    getIdealOuterLabelPositions: function(pie, i) {
        var labelGroupNode = pie.svg.select("#" + pie.cssPrefix + "labelGroup" + i + "-outer").node();
        if (!labelGroupNode) return;

        var labelGroupDims = labelGroupNode.getBBox();
        var angle = segments.getSegmentAngle(i, pie.options.data.content, pie.totalSize, { midpoint: true });

        var originalX = pie.pieCenter.x;
        var originalY = pie.pieCenter.y - (pie.outerRadius + pie.options.labels.outer.pieDistance);
        var newCoords = math.rotate(originalX, originalY, pie.pieCenter.x, pie.pieCenter.y, angle);

        // if the label is on the left half of the pie, adjust the values
        var hemisphere = "right"; // hemisphere
        if (angle > 180) {
            newCoords.x -= (labelGroupDims.width + 8);
            hemisphere = "left";
        } else {
            newCoords.x += 8;
        }

        pie.outerLabelGroupData[i] = {
            x: newCoords.x,
            y: newCoords.y,
            w: labelGroupDims.width,
            h: labelGroupDims.height,
            hs: hemisphere
        };
    }
};

    //// --------- segments.js -----------
var segments = {

    effectMap: {
        "none": d3.easeLinear,
        "bounce": d3.easeBounce,
        "linear": d3.easeLinear,
        "sin": d3.easeSin,
        "elastic": d3.easeElastic,
        "back": d3.easeBack,
        "quad": d3.easeQuad,
        "circle": d3.easeCircle,
        "exp": d3.easeExp
    },

    /**
     * Creates the pie chart segments and displays them according to the desired load effect.
     * @private
     */
    create: function(pie) {
        var pieCenter = pie.pieCenter;
        var colors = pie.options.colors;
        var loadEffects = pie.options.effects.load;
        var segmentStroke = pie.options.misc.colors.segmentStroke;

        // we insert the pie chart BEFORE the title, to ensure the title overlaps the pie
        var pieChartElement = pie.svg.insert("g", "#" + pie.cssPrefix + "title")
            .attr("transform", function() { return math.getPieTranslateCenter(pieCenter); })
            .attr("class", pie.cssPrefix + "pieChart");

        var arc = d3.arc()
            .innerRadius(pie.innerRadius)
            .outerRadius(pie.outerRadius)
            .startAngle(0)
            .endAngle(function(d) {
                return (d.value / pie.totalSize) * 2 * Math.PI;
            });

        var g = pieChartElement.selectAll("." + pie.cssPrefix + "arc")
            .data(pie.options.data.content)
            .enter()
            .append("g")
            .attr("class", pie.cssPrefix + "arc");

        // if we're not fading in the pie, just set the load speed to 0
        //var loadSpeed = loadEffects.speed;
        //if (loadEffects.effect === "none") {
        //	loadSpeed = 0;
        //}

        g.append("path")
            .attr("id", function(d, i) { return pie.cssPrefix + "segment" + i; })
            .attr("fill", function(d, i) {
                var color = colors[i];
                if (pie.options.misc.gradient.enabled) {
                    color = "url(#" + pie.cssPrefix + "grad" + i + ")";
                }
                return color;
            })
            .style("stroke", segmentStroke)
            .style("stroke-width", 1)
            //.transition()
            //.ease(d3.easeCubicInOut)
            //.duration(loadSpeed)
            .attr("data-index", function(d, i) { return i; })
            .attr("d", arc);
/*
            .attrTween("d", function(b) {
                var i = d3.interpolate({ value: 0 }, b);
                return function(t) {
                    var ret = pie.arc(i(t));
                    console.log(ret);
                    return ret;
                };
            });
*/
        pie.svg.selectAll("g." + pie.cssPrefix + "arc")
            .attr("transform",
            function(d, i) {
                var angle = 0;
                if (i > 0) {
                    angle = segments.getSegmentAngle(i-1, pie.options.data.content, pie.totalSize);
                }
                return "rotate(" + angle + ")";
            }
        );
        pie.arc = arc;
    },

    addGradients: function(pie) {
        var grads = pie.svg.append("defs")
            .selectAll("radialGradient")
            .data(pie.options.data.content)
            .enter().append("radialGradient")
            .attr("gradientUnits", "userSpaceOnUse")
            .attr("cx", 0)
            .attr("cy", 0)
            .attr("r", "120%")
            .attr("id", function(d, i) { return pie.cssPrefix + "grad" + i; });

        grads.append("stop").attr("offset", "0%").style("stop-color", function(d, i) { return pie.options.colors[i]; });
        grads.append("stop").attr("offset", pie.options.misc.gradient.percentage + "%").style("stop-color", pie.options.misc.gradient.color);
    },

    addSegmentEventHandlers: function(pie) {
        var arc = pie.svg.selectAll("." + pie.cssPrefix + "arc");
        arc = arc.merge(pie.__labels.inner.merge(pie.__labels.outer));

        arc.on("click", function() {
            var currentEl = d3.select(this);
            var segment;

            // mouseover works on both the segments AND the segment labels, hence the following
            if (currentEl.attr("class") === pie.cssPrefix + "arc") {
                segment = currentEl.select("path");
            } else {
                var index = currentEl.attr("data-index");
                segment = d3.select("#" + pie.cssPrefix + "segment" + index);
            }

            var isExpanded = segment.attr("class") === pie.cssPrefix + "expanded";
            segments.onSegmentEvent(pie, pie.options.callbacks.onClickSegment, segment, isExpanded);
            if (pie.options.effects.pullOutSegmentOnClick.effect !== "none") {
                if (isExpanded) {
                    segments.closeSegment(pie, segment.node());
                } else {
                    segments.openSegment(pie, segment.node());
                }
            }
        });

        arc.on("mouseover", function() {
            var currentEl = d3.select(this);
            var segment, index;

            if (currentEl.attr("class") === pie.cssPrefix + "arc") {
                segment = currentEl.select("path");
            } else {
                index = currentEl.attr("data-index");
                segment = d3.select("#" + pie.cssPrefix + "segment" + index);
            }

            if (pie.options.effects.highlightSegmentOnMouseover) {
                index = segment.attr("data-index");
                var segColor = pie.options.colors[index];
                segment.style("fill", helpers.getColorShade(segColor, pie.options.effects.highlightLuminosity));
            }

            if (pie.options.tooltips.enabled) {
                index = segment.attr("data-index");
                tt.showTooltip(pie, index);
            }

            var isExpanded = segment.attr("class") === pie.cssPrefix + "expanded";
            segments.onSegmentEvent(pie, pie.options.callbacks.onMouseoverSegment, segment, isExpanded);
        });

        arc.on("mousemove", function() {
            tt.moveTooltip(pie);
        });

        arc.on("mouseout", function() {
            var currentEl = d3.select(this);
            var segment, index;

            if (currentEl.attr("class") === pie.cssPrefix + "arc") {
                segment = currentEl.select("path");
            } else {
                index = currentEl.attr("data-index");
                segment = d3.select("#" + pie.cssPrefix + "segment" + index);
            }

            if (pie.options.effects.highlightSegmentOnMouseover) {
                index = segment.attr("data-index");
                var color = pie.options.colors[index];
                if (pie.options.misc.gradient.enabled) {
                    color = "url(#" + pie.cssPrefix + "grad" + index + ")";
                }
                segment.style("fill", color);
            }

            if (pie.options.tooltips.enabled) {
                index = segment.attr("data-index");
                tt.hideTooltip(pie, index);
            }

            var isExpanded = segment.attr("class") === pie.cssPrefix + "expanded";
            segments.onSegmentEvent(pie, pie.options.callbacks.onMouseoutSegment, segment, isExpanded);
        });
    },

    // helper function used to call the click, mouseover, mouseout segment callback functions
    onSegmentEvent: function(pie, func, segment, isExpanded) {
        if (!helpers.isFunction(func)) {
            return;
        }
        var index = parseInt(segment.attr("data-index"), 10);
        func({
            segment: segment.node(),
            index: index,
            expanded: isExpanded,
            data: pie.options.data.content[index]
        });
    },

    openSegment: function(pie, segment) {
        if (pie.isOpeningSegment) {
            return;
        }
        pie.isOpeningSegment = true;

        segments.maybeCloseOpenSegment(pie);

        d3.select(segment)
            .transition()
            .ease(segments.effectMap[pie.options.effects.pullOutSegmentOnClick.effect])
            .duration(pie.options.effects.pullOutSegmentOnClick.speed)
            .attr("transform", function(d, i) {
                var c = pie.arc.centroid(d),
                    x = c[0],
                    y = c[1],
                    h = Math.sqrt(x*x + y*y),
                    pullOutSize = parseInt(pie.options.effects.pullOutSegmentOnClick.size, 10);

                return "translate(" + ((x/h) * pullOutSize) + ',' + ((y/h) * pullOutSize) + ")";
            })
            .on("end", function(d, i) {
                pie.currentlyOpenSegment = segment;
                pie.isOpeningSegment = false;
                d3.select(segment).attr("class", pie.cssPrefix + "expanded");
            });
    },

    maybeCloseOpenSegment: function(pie) {
        if (typeof pie !== 'undefined' && pie.svg.selectAll("." + pie.cssPrefix + "expanded").size() > 0) {
            segments.closeSegment(pie, pie.svg.select("." + pie.cssPrefix + "expanded").node());
        }
    },

    closeSegment: function(pie, segment) {
        d3.select(segment)
            .transition()
            .duration(400)
            .attr("transform", "translate(0,0)")
            .on("end", function(d, i) {
                d3.select(segment).attr("class", "");
                pie.currentlyOpenSegment = null;
            });
    },

    getCentroid: function(el) {
        var bbox = el.getBBox();
        return {
            x: bbox.x + bbox.width / 2,
            y: bbox.y + bbox.height / 2
        };
    },

    /**
     * General helper function to return a segment's angle, in various different ways.
     * @param index
     * @param opts optional object for fine-tuning exactly what you want.
     */
    getSegmentAngle: function(index, data, totalSize, opts) {
        var options = extend({
            // if true, this returns the full angle from the origin. Otherwise it returns the single segment angle
            compounded: true,

            // optionally returns the midpoint of the angle instead of the full angle
            midpoint: false
        }, opts);

        var currValue = data[index].value;
        var fullValue;
        if (options.compounded) {
            fullValue = 0;

            // get all values up to and including the specified index
            for (var i=0; i<=index; i++) {
                fullValue += data[i].value;
            }
        }

        if (typeof fullValue === 'undefined') {
            fullValue = currValue;
        }

        // now convert the full value to an angle
        var angle = (fullValue / totalSize) * 360;

        // lastly, if we want the midpoint, factor that sucker in
        if (options.midpoint) {
            var currAngle = (currValue / totalSize) * 360;
            angle -= (currAngle / 2);
        }

        return angle;
    }

};

    //// --------- text.js -----------
var text = {
    offscreenCoord: -10000,

    addTitle: function(pie) {
        pie.__title = pie.svg.selectAll("." + pie.cssPrefix + "title")
            .data([pie.options.header.title])
            .enter()
            .append("text")
            .text(function(d) { return d.text; })
            .attr("id", pie.cssPrefix + "title")
            .attr("class", pie.cssPrefix + "title")
            .attr("x", text.offscreenCoord)
            .attr("y", text.offscreenCoord)
            .attr("text-anchor", function() {
                var location;
                if (pie.options.header.location === "top-center" || pie.options.header.location === "pie-center") {
                    location = "middle";
                } else {
                    location = "left";
                }
                return location;
            })
            .attr("fill", function(d) { return d.color; })
            .style("font-size", function(d) { return d.fontSize + "px"; })
            .style("font-weight", function(d) { return d.fontWeight; })
            .style("font-family", function(d) { return d.font; });
    },

    positionTitle: function(pie) {
        var textComponents = pie.textComponents;
        var headerLocation = pie.options.header.location;
        var canvasPadding = pie.options.misc.canvasPadding;
        var canvasWidth = pie.options.size.canvasWidth;
        var titleSubtitlePadding = pie.options.header.titleSubtitlePadding;

        var x;
        if (headerLocation === "top-left") {
            x = canvasPadding.left;
        } else {
            x = ((canvasWidth - canvasPadding.right) / 2) + canvasPadding.left;
        }

        // add whatever offset has been added by user
        x += pie.options.misc.pieCenterOffset.x;

        var y = canvasPadding.top + textComponents.title.h;

        if (headerLocation === "pie-center") {
            y = pie.pieCenter.y;

            // still not fully correct
            if (textComponents.subtitle.exists) {
                var totalTitleHeight = textComponents.title.h + titleSubtitlePadding + textComponents.subtitle.h;
                y = y - (totalTitleHeight / 2) + textComponents.title.h;
            } else {
                y += (textComponents.title.h / 4);
            }
        }

        pie.__title
            .attr("x", x)
            .attr("y", y);
    },

    addSubtitle: function(pie) {
        var headerLocation = pie.options.header.location;

        pie.__subtitle = pie.svg.selectAll("." + pie.cssPrefix + "subtitle")
            .data([pie.options.header.subtitle])
            .enter()
            .append("text")
            .text(function(d) { return d.text; })
            .attr("x", text.offscreenCoord)
            .attr("y", text.offscreenCoord)
            .attr("id", pie.cssPrefix + "subtitle")
            .attr("class", pie.cssPrefix + "subtitle")
            .attr("text-anchor", function() {
                var location;
                if (headerLocation === "top-center" || headerLocation === "pie-center") {
                    location = "middle";
                } else {
                    location = "left";
                }
                return location;
            })
            .attr("fill", function(d) { return d.color; })
            .style("font-size", function(d) { return d.fontSize + "px"; })
            .style("font-weight", function(d) { return d.fontWeight; })
            .style("font-family", function(d) { return d.font; });
    },

    positionSubtitle: function(pie) {
        var canvasPadding = pie.options.misc.canvasPadding;
        var canvasWidth = pie.options.size.canvasWidth;

        var x;
        if (pie.options.header.location === "top-left") {
            x = canvasPadding.left;
        } else {
            x = ((canvasWidth - canvasPadding.right) / 2) + canvasPadding.left;
        }

        // add whatever offset has been added by user
        x += pie.options.misc.pieCenterOffset.x;

        var y = text.getHeaderHeight(pie);

        pie.__subtitle
            .attr("x", x)
            .attr("y", y);
    },

    addFooter: function(pie) {
        pie.__footer = pie.svg.selectAll("." + pie.cssPrefix + "footer")
            .data([pie.options.footer])
            .enter()
            .append("text")
            .text(function(d) { return d.text; })
            .attr("x", text.offscreenCoord)
            .attr("y", text.offscreenCoord)
            .attr("id", pie.cssPrefix + "footer")
            .attr("class", pie.cssPrefix + "footer")
            .attr("text-anchor", function() {
                var location = "left";
                if (pie.options.footer.location === "bottom-center") {
                    location = "middle";
                } else if (pie.options.footer.location === "bottom-right") {
                    location = "left"; // on purpose. We have to change the x-coord to make it properly right-aligned
                }
                return location;
            })
            .attr("fill", function(d) { return d.color; })
            .style("font-size", function(d) { return d.fontSize + "px"; })
            .style("font-weight", function(d) { return d.fontWeight; })
            .style("font-family", function(d) { return d.font; });
    },

    positionFooter: function(pie) {
        var footerLocation = pie.options.footer.location;
        var footerWidth = pie.textComponents.footer.w;
        var canvasWidth = pie.options.size.canvasWidth;
        var canvasHeight = pie.options.size.canvasHeight;
        var canvasPadding = pie.options.misc.canvasPadding;

        var x;
        if (footerLocation === "bottom-left") {
            x = canvasPadding.left;
        } else if (footerLocation === "bottom-right") {
            x = canvasWidth - footerWidth - canvasPadding.right;
        } else {
            x = canvasWidth / 2; // TODO - shouldn't this also take into account padding?
        }

        pie.__footer
            .attr("x", x)
            .attr("y", canvasHeight - canvasPadding.bottom);
    },

    getHeaderHeight: function(pie) {
        var h;
        if (pie.textComponents.title.exists) {

            // if the subtitle isn't defined, it'll be set to 0
            var totalTitleHeight = pie.textComponents.title.h + pie.options.header.titleSubtitlePadding + pie.textComponents.subtitle.h;
            if (pie.options.header.location === "pie-center") {
                h = pie.pieCenter.y - (totalTitleHeight / 2) + totalTitleHeight;
            } else {
                h = totalTitleHeight + pie.options.misc.canvasPadding.top;
            }
        } else {
            if (pie.options.header.location === "pie-center") {
                var footerPlusPadding = pie.options.misc.canvasPadding.bottom + pie.textComponents.footer.h;
                h = ((pie.options.size.canvasHeight - footerPlusPadding) / 2) + pie.options.misc.canvasPadding.top + (pie.textComponents.subtitle.h / 2);
            } else {
                h = pie.options.misc.canvasPadding.top + pie.textComponents.subtitle.h;
            }
        }
        return h;
    }
};

    //// --------- validate.js -----------
var tt = {
    addTooltips: function(pie) {

    // group the label groups (label, percentage, value) into a single element for simpler positioning
    var tooltips = pie.svg.insert("g")
        .attr("class", pie.cssPrefix + "tooltips");

    tooltips.selectAll("." + pie.cssPrefix + "tooltip")
        .data(pie.options.data.content)
        .enter()
        .append("g")
        .attr("class", pie.cssPrefix + "tooltip")
        .attr("id", function(d, i) { return pie.cssPrefix + "tooltip" + i; })
        .style("opacity", 0)
        .append("rect")
        .attr("rx", pie.options.tooltips.styles.borderRadius)
        .attr("ry", pie.options.tooltips.styles.borderRadius)
        .attr("x", -pie.options.tooltips.styles.padding)
        .attr("opacity", pie.options.tooltips.styles.backgroundOpacity)
        .style("fill", pie.options.tooltips.styles.backgroundColor);

    tooltips.selectAll("." + pie.cssPrefix + "tooltip")
        .data(pie.options.data.content)
        .append("text")
        .attr("fill", function(d) { return pie.options.tooltips.styles.color; })
        .style("font-size", function(d) { return pie.options.tooltips.styles.fontSize; })
        .style("font-weight", function(d) { return pie.options.tooltips.styles.fontWeight; })
        .style("font-family", function(d) { return pie.options.tooltips.styles.font; })
        .text(function(d, i) {
            var caption = pie.options.tooltips.string;
            if (pie.options.tooltips.type === "caption") {
                caption = d.caption;
            }
            return tt.replacePlaceholders(pie, caption, i, {
                label: d.label,
                value: d.value,
                percentage: d.percentage
            });
        });

        tooltips.selectAll("." + pie.cssPrefix + "tooltip rect")
            .attr("width", function (d, i) {
                var dims = helpers.getDimensions(pie.cssPrefix + "tooltip" + i);
                return dims.w + (2 * pie.options.tooltips.styles.padding);
            })
            .attr("height", function (d, i) {
                var dims = helpers.getDimensions(pie.cssPrefix + "tooltip" + i);
                return dims.h + (2 * pie.options.tooltips.styles.padding);
            })
            .attr("y", function (d, i) {
                var dims = helpers.getDimensions(pie.cssPrefix + "tooltip" + i);
                return -(dims.h / 2) + 1;
            });
    },

    showTooltip: function(pie, index) {
        var fadeInSpeed = pie.options.tooltips.styles.fadeInSpeed;
        if (tt.currentTooltip === index) {
            fadeInSpeed = 1;
        }

        tt.currentTooltip = index;
        d3.select("#" + pie.cssPrefix + "tooltip" + index)
            .transition()
            .duration(fadeInSpeed)
            .style("opacity", function() { return 1; });

        tt.moveTooltip(pie);
    },

    moveTooltip: function(pie) {
        d3.selectAll("#" + pie.cssPrefix + "tooltip" + tt.currentTooltip)
            .attr("transform", function(d) {
                var mouseCoords = d3.mouse(this.parentNode);
                var x = mouseCoords[0] + pie.options.tooltips.styles.padding + 2;
                var y = mouseCoords[1] - (2 * pie.options.tooltips.styles.padding) - 2;
                    return "translate(" + x + "," + y + ")";
                });
    },

    hideTooltip: function(pie, index) {
        d3.select("#" + pie.cssPrefix + "tooltip" + index)
            .style("opacity", function() { return 0; });

        // move the tooltip offscreen. This ensures that when the user next mouseovers the segment the hidden
        // element won't interfere
        d3.select("#" + pie.cssPrefix + "tooltip" + tt.currentTooltip)
            .attr("transform", function(d, i) {
                // klutzy, but it accounts for tooltip padding which could push it onscreen
                var x = pie.options.size.canvasWidth + 1000;
                var y = pie.options.size.canvasHeight + 1000;
                return "translate(" + x + "," + y + ")";
            });
    },

    replacePlaceholders: function(pie, str, index, replacements) {

        // if the user has defined a placeholderParser function, call it before doing the replacements
        if (helpers.isFunction(pie.options.tooltips.placeholderParser)) {
            pie.options.tooltips.placeholderParser(index, replacements);
        }

        var replacer = function()  {
            return function(match) {
                var placeholder = arguments[1];
                if (replacements.hasOwnProperty(placeholder)) {
                    return replacements[arguments[1]];
                } else {
                    return arguments[0];
                }
            };
        };
        return str.replace(/\{(\w+)\}/g, replacer(replacements));
    }
};


    // --------------------------------------------------------------------------------------------

    // our constructor
    var d3pie = function(element, options) {

        // element can be an ID or DOM element
        this.element = element;
        if (typeof element === "string") {
            var el = element.replace(/^#/, ""); // replace any jQuery-like ID hash char
            this.element = document.getElementById(el);
        }

        var opts = {};
        extend(true, opts, defaultSettings, options);
        this.options = opts;

        // if the user specified a custom CSS element prefix (ID, class), use it
        if (this.options.misc.cssPrefix !== null) {
            this.cssPrefix = this.options.misc.cssPrefix;
        } else {
            this.cssPrefix = "p" + _uniqueIDCounter + "_";
            _uniqueIDCounter++;
        }


        // now run some validation on the user-defined info
        if (!validate.initialCheck(this)) {
            return;
        }

        // add a data-role to the DOM node to let anyone know that it contains a d3pie instance, and the d3pie version
        d3.select(this.element).attr(_scriptName, _version);

        // things that are done once
        _setupData.call(this);
        _init.call(this);
    };

    d3pie.prototype.recreate = function() {
        // now run some validation on the user-defined info
        if (!validate.initialCheck(this)) {
            return;
        }

        _setupData.call(this);
        _init.call(this);
    };

    d3pie.prototype.redraw = function() {
        this.element.innerHTML = "";
        _init.call(this);
    };

    d3pie.prototype.destroy = function() {
        this.element.innerHTML = ""; // clear out the SVG
        d3.select(this.element).attr(_scriptName, null); // remove the data attr
    };

    /**
     * Returns all pertinent info about the current open info. Returns null if nothing's open, or if one is, an object of
     * the following form:
     * 	{
     * 	  element: DOM NODE,
     * 	  index: N,
     * 	  data: {}
     * 	}
     */
    d3pie.prototype.getOpenSegment = function() {
        var segment = this.currentlyOpenSegment;
        if (segment !== null && typeof segment !== "undefined") {
            var index = parseInt(d3.select(segment).attr("data-index"), 10);
            return {
                element: segment,
                index: index,
                data: this.options.data.content[index]
            };
        } else {
            return null;
        }
    };

    d3pie.prototype.openSegment = function(index) {
        index = parseInt(index, 10);
        if (index < 0 || index > this.options.data.content.length-1) {
            return;
        }
        segments.openSegment(this, d3.select("#" + this.cssPrefix + "segment" + index).node());
    };

    d3pie.prototype.closeSegment = function() {
        segments.maybeCloseOpenSegment(this);
    };

    // this let's the user dynamically update aspects of the pie chart without causing a complete redraw. It
    // intelligently re-renders only the part of the pie that the user specifies. Some things cause a repaint, others
    // just redraw the single element
    d3pie.prototype.updateProp = function(propKey, value) {
        switch (propKey) {
            case "header.title.text":
                var oldVal = helpers.processObj(this.options, propKey);
                helpers.processObj(this.options, propKey, value);
                d3.select("#" + this.cssPrefix + "title").html(value);
                if ((oldVal === "" && value !== "") || (oldVal !== "" && value === "")) {
                    this.redraw();
                }
                break;

            case "header.subtitle.text":
                var oldValue = helpers.processObj(this.options, propKey);
                helpers.processObj(this.options, propKey, value);
                d3.select("#" + this.cssPrefix + "subtitle").html(value);
                if ((oldValue === "" && value !== "") || (oldValue !== "" && value === "")) {
                    this.redraw();
                }
                break;

            case "callbacks.onload":
            case "callbacks.onMouseoverSegment":
            case "callbacks.onMouseoutSegment":
            case "callbacks.onClickSegment":
            case "effects.pullOutSegmentOnClick.effect":
            case "effects.pullOutSegmentOnClick.speed":
            case "effects.pullOutSegmentOnClick.size":
            case "effects.highlightSegmentOnMouseover":
            case "effects.highlightLuminosity":
                helpers.processObj(this.options, propKey, value);
                break;

            // everything else, attempt to update it & do a repaint
            default:
                helpers.processObj(this.options, propKey, value);

                this.destroy();
                this.recreate();
                break;
        }
    };


    // ------------------------------------------------------------------------------------------------

    var _setupData = function () {
        this.options.data.content = math.sortPieData(this);
        if (this.options.data.smallSegmentGrouping.enabled) {
            this.options.data.content = helpers.applySmallSegmentGrouping(this.options.data.content, this.options.data.smallSegmentGrouping);
        }


        this.options.colors = helpers.initSegmentColors(this);
        this.totalSize      = math.getTotalPieSize(this.options.data.content);

        var dp = this.options.labels.percentage.decimalPlaces;

        // add in percentage data to content
        for (var i=0; i<this.options.data.content.length; i++) {
            this.options.data.content[i].percentage = _getPercentage(this.options.data.content[i].value, this.totalSize, dp);
        }

        // adjust the final item to ensure the percentage always adds up to precisely 100%. This is necessary
        var totalPercentage = 0;
        for (var j=0; j<this.options.data.content.length; j++) {
            if (j === this.options.data.content.length - 1) {
                this.options.data.content[j].percentage = (100 - totalPercentage).toFixed(dp);
            }
            totalPercentage += parseFloat(this.options.data.content[j].percentage);
        }
    };

    var _init = function() {

        // prep-work
        this.svg = helpers.addSVGSpace(this);

        // store info about the main text components as part of the d3pie object instance
        this.textComponents = {
            headerHeight: 0,
            title: {
                exists: this.options.header.title.text !== "",
                h: 0,
                w: 0
            },
            subtitle: {
                exists: this.options.header.subtitle.text !== "",
                h: 0,
                w: 0
            },
            footer: {
                exists: this.options.footer.text !== "",
                h: 0,
                w: 0
            }
        };

        this.outerLabelGroupData = [];

        // add the key text components offscreen (title, subtitle, footer). We need to know their widths/heights for later computation
        if (this.textComponents.title.exists) text.addTitle(this);
        if (this.textComponents.subtitle.exists) text.addSubtitle(this);
        text.addFooter(this);

        // console.log(this);

        // the footer never moves. Put it in place now
        var self = this;
        text.positionFooter(self);
        var d3 = helpers.getDimensions(self.__footer.node());
        self.textComponents.footer.h = d3.h;
        self.textComponents.footer.w = d3.w;

        if (self.textComponents.title.exists) {
            var d1 = helpers.getDimensions(self.__title.node());
            self.textComponents.title.h = d1.h;
            self.textComponents.title.w = d1.w;
        }

        if (self.textComponents.subtitle.exists) {
            var d2 = helpers.getDimensions(self.__subtitle.node());
            self.textComponents.subtitle.h = d2.h;
            self.textComponents.subtitle.w = d2.w;
        }

        // now compute the full header height
        if (self.textComponents.title.exists || self.textComponents.subtitle.exists) {
            var headerHeight = 0;
            if (self.textComponents.title.exists) {
                headerHeight += self.textComponents.title.h;
                if (self.textComponents.subtitle.exists) {
                    headerHeight += self.options.header.titleSubtitlePadding;
                }
            }
            if (self.textComponents.subtitle.exists) {
                headerHeight += self.textComponents.subtitle.h;
            }
            self.textComponents.headerHeight = headerHeight;
        }

        // at this point, all main text component dimensions have been calculated
        math.computePieRadius(self);

        // this value is used all over the place for placing things and calculating locations. We figure it out ONCE
        // and store it as part of the object
        math.calculatePieCenter(self);

        // position the title and subtitle
        text.positionTitle(self);
        text.positionSubtitle(self);

        // now create the pie chart segments, and gradients if the user desired
        if (self.options.misc.gradient.enabled) {
            segments.addGradients(self);
        }
        segments.create(self); // also creates this.arc

        self.__labels = {};
        labels.add(self, "inner", self.options.labels.inner.format);
        labels.add(self, "outer", self.options.labels.outer.format);

        // position the label elements relatively within their individual group (label, percentage, value)
        labels.positionLabelElements(self, "inner", self.options.labels.inner.format);
        labels.positionLabelElements(self, "outer", self.options.labels.outer.format);
        labels.computeOuterLabelCoords(self);

        // this is (and should be) dumb. It just places the outer groups at their calculated, collision-free positions
        labels.positionLabelGroups(self, "outer");

        // we use the label line positions for many other calculations, so ALWAYS compute them
        labels.computeLabelLinePositions(self);

        // only add them if they're actually enabled
        if (self.options.labels.lines.enabled && self.options.labels.outer.format !== "none") {
            labels.addLabelLines(self);
        }

        labels.positionLabelGroups(self, "inner");

        if (helpers.isFunction(self.options.callbacks.onload)) {
            try {
                self.options.callbacks.onload();
            } catch (e) { }
        }

        // add and position the tooltips
        if (self.options.tooltips.enabled) {
            tt.addTooltips(self);
        }

        segments.addSegmentEventHandlers(self);
    };

    var _getPercentage = function(value, total, decimalPlaces) {
        var relativeAmount = value / total;
        if (decimalPlaces <= 0) {
            return Math.round(relativeAmount * 100);
        } else {
            return (relativeAmount * 100).toFixed(decimalPlaces);
        }
    };

    return d3pie;
}));
