/**
 * Synchronize zooming and/or selections between a set of dygraphs.
 *
 * Usage:
 *
 *   var g1 = new Dygraph(...),
 *       g2 = new Dygraph(...),
 *       ...;
 *   var sync = Dygraph.synchronize(g1, g2, ...);
 *   // charts are now synchronized
 *   sync.detach();
 *   // charts are no longer synchronized
 *
 * You can set options using the last parameter, for example:
 *
 *   var sync = Dygraph.synchronize(g1, g2, g3, {
 *      selection: true,
 *      zoom: true
 *   });
 *
 * The default is to synchronize both of these.
 *
 * Instead of passing one Dygraph object as each parameter, you may also pass an
 * array of dygraphs:
 *
 *   var sync = Dygraph.synchronize([g1, g2, g3], {
 *      selection: false,
 *      zoom: true
 *   });
 *
 * You may also set `range: false` if you wish to only sync the x-axis.
 * The `range` option has no effect unless `zoom` is true (the default).
 */
(function() {
/* global Dygraph:false */
'use strict';

var Dygraph;
if (window.Dygraph) {
  Dygraph = window.Dygraph;
} else if (typeof(module) !== 'undefined') {
  Dygraph = require('../dygraph');
}

var synchronize = function(/* dygraphs..., opts */) {
  if (arguments.length === 0) {
    throw 'Invalid invocation of Dygraph.synchronize(). Need >= 1 argument.';
  }

  var OPTIONS = ['selection', 'zoom', 'range'];
  var opts = {
    selection: true,
    zoom: true,
    range: true
  };
  var dygraphs = [];
  var prevCallbacks = [];

  var parseOpts = function(obj) {
    if (!(obj instanceof Object)) {
      throw 'Last argument must be either Dygraph or Object.';
    } else {
      for (var i = 0; i < OPTIONS.length; i++) {
        var optName = OPTIONS[i];
        if (obj.hasOwnProperty(optName)) opts[optName] = obj[optName];
      }
    }
  };

  if (arguments[0] instanceof Dygraph) {
    // Arguments are Dygraph objects.
    for (var i = 0; i < arguments.length; i++) {
      if (arguments[i] instanceof Dygraph) {
        dygraphs.push(arguments[i]);
      } else {
        break;
      }
    }
    if (i < arguments.length - 1) {
      throw 'Invalid invocation of Dygraph.synchronize(). ' +
            'All but the last argument must be Dygraph objects.';
    } else if (i == arguments.length - 1) {
      parseOpts(arguments[arguments.length - 1]);
    }
  } else if (arguments[0].length) {
    // Invoked w/ list of dygraphs, options
    for (var i = 0; i < arguments[0].length; i++) {
      dygraphs.push(arguments[0][i]);
    }
    if (arguments.length == 2) {
      parseOpts(arguments[1]);
    } else if (arguments.length > 2) {
      throw 'Invalid invocation of Dygraph.synchronize(). ' +
            'Expected two arguments: array and optional options argument.';
    }  // otherwise arguments.length == 1, which is fine.
  } else {
    throw 'Invalid invocation of Dygraph.synchronize(). ' +
          'First parameter must be either Dygraph or list of Dygraphs.';
  }

  if (dygraphs.length < 2) {
    throw 'Invalid invocation of Dygraph.synchronize(). ' +
          'Need two or more dygraphs to synchronize.';
  }

  var readycount = dygraphs.length;
  for (var i = 0; i < dygraphs.length; i++) {
    var g = dygraphs[i];
    g.ready( function() {
      if (--readycount == 0) {
        // store original callbacks
        var callBackTypes = ['drawCallback', 'highlightCallback', 'unhighlightCallback'];
        for (var j = 0; j < dygraphs.length; j++) {
          if (!prevCallbacks[j]) {
            prevCallbacks[j] = {};
          }
          for (var k = callBackTypes.length - 1; k >= 0; k--) {
            prevCallbacks[j][callBackTypes[k]] = dygraphs[j].getFunctionOption(callBackTypes[k]);
          }
        }

        // Listen for draw, highlight, unhighlight callbacks.
        if (opts.zoom) {
          attachZoomHandlers(dygraphs, opts, prevCallbacks);
        }

        if (opts.selection) {
          attachSelectionHandlers(dygraphs, prevCallbacks);
        }
      }
    });
  }

  return {
    detach: function() {
      for (var i = 0; i < dygraphs.length; i++) {
        var g = dygraphs[i];
        if (opts.zoom) {
          g.updateOptions({drawCallback: prevCallbacks[i].drawCallback});
        }
        if (opts.selection) {
          g.updateOptions({
            highlightCallback: prevCallbacks[i].highlightCallback,
            unhighlightCallback: prevCallbacks[i].unhighlightCallback
          });
        }
      }
      // release references & make subsequent calls throw.
      dygraphs = null;
      opts = null;
      prevCallbacks = null;
    }
  };
};

function attachZoomHandlers(gs, syncOpts, prevCallbacks) {
  var block = false;
  for (var i = 0; i < gs.length; i++) {
    var g = gs[i];
    g.updateOptions({
      drawCallback: function(me, initial) {
        if (block || initial) return;
        block = true;
        var opts = {
          dateWindow: me.xAxisRange()
        };
        if (syncOpts.range) opts.valueRange = me.yAxisRange();

        for (var j = 0; j < gs.length; j++) {
          if (gs[j] == me) {
            if (prevCallbacks[j] && prevCallbacks[j].drawCallback) {
              prevCallbacks[j].drawCallback.apply(this, arguments);
            }
            continue;
          }
          gs[j].updateOptions(opts);
        }
        block = false;
      }
    }, true /* no need to redraw */);
  }
}

function attachSelectionHandlers(gs, prevCallbacks) {
  var block = false;
  for (var i = 0; i < gs.length; i++) {
    var g = gs[i];

    g.updateOptions({
      highlightCallback: function(event, x, points, row, seriesName) {
        if (block) return;
        block = true;
        var me = this;
        for (var i = 0; i < gs.length; i++) {
          if (me == gs[i]) {
            if (prevCallbacks[i] && prevCallbacks[i].highlightCallback) {
              prevCallbacks[i].highlightCallback.apply(this, arguments);
            }
            continue;
          }
          var idx = gs[i].getRowForX(x);
          if (idx !== null) {
            gs[i].setSelection(idx, seriesName);
          }
        }
        block = false;
      },
      unhighlightCallback: function(event) {
        if (block) return;
        block = true;
        var me = this;
        for (var i = 0; i < gs.length; i++) {
          if (me == gs[i]) {
            if (prevCallbacks[i] && prevCallbacks[i].unhighlightCallback) {
              prevCallbacks[i].unhighlightCallback.apply(this, arguments);
            }
            continue;
          }
          gs[i].clearSelection();
        }
        block = false;
      }
    }, true /* no need to redraw */);
  }
}

Dygraph.synchronize = synchronize;

})();
