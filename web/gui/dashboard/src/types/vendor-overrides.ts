import Dygraph from "dygraphs"

export declare class NetdataDygraph extends Dygraph {
  /**
   * Returns the lower- and upper-bound y-axis values for each axis.
   */
  yAxisExtremes(): [[number, number]];
}

export interface DygraphArea {
  x: number
  y: number
  w: number
  h: number
}
