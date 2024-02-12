// SPDX-License-Identifier: MIT
(function() {
"use strict";

var Dygraph;
if (window.Dygraph) {
  Dygraph = window.Dygraph;
} else if (typeof(module) !== 'undefined') {
  Dygraph = require('../dygraph');
}

/**
 * Given three sequential points, p0, p1 and p2, find the left and right
 * control points for p1.
 *
 * The three points are expected to have x and y properties.
 *
 * The alpha parameter controls the amount of smoothing.
 * If α=0, then both control points will be the same as p1 (i.e. no smoothing).
 *
 * Returns [l1x, l1y, r1x, r1y]
 *
 * It's guaranteed that the line from (l1x, l1y)-(r1x, r1y) passes through p1.
 * Unless allowFalseExtrema is set, then it's also guaranteed that:
 *   l1y ∈ [p0.y, p1.y]
 *   r1y ∈ [p1.y, p2.y]
 *
 * The basic algorithm is:
 * 1. Put the control points l1 and r1 α of the way down (p0, p1) and (p1, p2).
 * 2. Shift l1 and r2 so that the line l1–r1 passes through p1
 * 3. Adjust to prevent false extrema while keeping p1 on the l1–r1 line.
 *
 * This is loosely based on the HighCharts algorithm.
 */
function getControlPoints(p0, p1, p2, opt_alpha, opt_allowFalseExtrema) {
  var alpha = (opt_alpha !== undefined) ? opt_alpha : 1/3;  // 0=no smoothing, 1=crazy smoothing
  var allowFalseExtrema = opt_allowFalseExtrema || false;

  if (!p2) {
    return [p1.x, p1.y, null, null];
  }

  // Step 1: Position the control points along each line segment.
  var l1x = (1 - alpha) * p1.x + alpha * p0.x,
      l1y = (1 - alpha) * p1.y + alpha * p0.y,
      r1x = (1 - alpha) * p1.x + alpha * p2.x,
      r1y = (1 - alpha) * p1.y + alpha * p2.y;

  // Step 2: shift the points up so that p1 is on the l1–r1 line.
  if (l1x != r1x) {
    // This can be derived w/ some basic algebra.
    var deltaY = p1.y - r1y - (p1.x - r1x) * (l1y - r1y) / (l1x - r1x);
    l1y += deltaY;
    r1y += deltaY;
  }

  // Step 3: correct to avoid false extrema.
  if (!allowFalseExtrema) {
    if (l1y > p0.y && l1y > p1.y) {
      l1y = Math.max(p0.y, p1.y);
      r1y = 2 * p1.y - l1y;
    } else if (l1y < p0.y && l1y < p1.y) {
      l1y = Math.min(p0.y, p1.y);
      r1y = 2 * p1.y - l1y;
    }

    if (r1y > p1.y && r1y > p2.y) {
      r1y = Math.max(p1.y, p2.y);
      l1y = 2 * p1.y - r1y;
    } else if (r1y < p1.y && r1y < p2.y) {
      r1y = Math.min(p1.y, p2.y);
      l1y = 2 * p1.y - r1y;
    }
  }

  return [l1x, l1y, r1x, r1y];
}

// i.e. is none of (null, undefined, NaN)
function isOK(x) {
  return !!x && !isNaN(x);
};

// A plotter which uses splines to create a smooth curve.
// See tests/plotters.html for a demo.
// Can be controlled via smoothPlotter.smoothing
function smoothPlotter(e) {
  var ctx = e.drawingContext,
      points = e.points;

  ctx.beginPath();
  ctx.moveTo(points[0].canvasx, points[0].canvasy);

  // right control point for previous point
  var lastRightX = points[0].canvasx, lastRightY = points[0].canvasy;

  for (var i = 1; i < points.length; i++) {
    var p0 = points[i - 1],
        p1 = points[i],
        p2 = points[i + 1];
    p0 = p0 && isOK(p0.canvasy) ? p0 : null;
    p1 = p1 && isOK(p1.canvasy) ? p1 : null;
    p2 = p2 && isOK(p2.canvasy) ? p2 : null;
    if (p0 && p1) {
      var controls = getControlPoints({x: p0.canvasx, y: p0.canvasy},
                                      {x: p1.canvasx, y: p1.canvasy},
                                      p2 && {x: p2.canvasx, y: p2.canvasy},
                                      smoothPlotter.smoothing);
      // Uncomment to show the control points:
      // ctx.lineTo(lastRightX, lastRightY);
      // ctx.lineTo(controls[0], controls[1]);
      // ctx.lineTo(p1.canvasx, p1.canvasy);
      lastRightX = (lastRightX !== null) ? lastRightX : p0.canvasx;
      lastRightY = (lastRightY !== null) ? lastRightY : p0.canvasy;
      ctx.bezierCurveTo(lastRightX, lastRightY,
                        controls[0], controls[1],
                        p1.canvasx, p1.canvasy);
      lastRightX = controls[2];
      lastRightY = controls[3];
    } else if (p1) {
      // We're starting again after a missing point.
      ctx.moveTo(p1.canvasx, p1.canvasy);
      lastRightX = p1.canvasx;
      lastRightY = p1.canvasy;
    } else {
      lastRightX = lastRightY = null;
    }
  }

  ctx.stroke();
}
smoothPlotter.smoothing = 1/3;
smoothPlotter._getControlPoints = getControlPoints;  // for testing

// older versions exported a global.
// This will be removed in the future.
// The preferred way to access smoothPlotter is via Dygraph.smoothPlotter.
window.smoothPlotter = smoothPlotter;
Dygraph.smoothPlotter = smoothPlotter;

})();
