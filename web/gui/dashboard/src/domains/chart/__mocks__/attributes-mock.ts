import { Attributes } from "../utils/transformDataAttributes"

export const attributesMock: Attributes = {
  id: "system.cpu",
  host: "http://localhost:19999/",
  title: "CPU usage of your netdata server",
  chartLibrary: "dygraph",
  width: "49%",
  height: "100%",
  after: -300,
  before: 0,
  legend: true,
  dygraphValueRange: [
    0,
    100,
  ],
}
