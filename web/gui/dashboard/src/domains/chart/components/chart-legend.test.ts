import { getNewSelectedDimensions } from "./chart-legend"

describe("chart-legend", () => {
  describe("getNewSelectedDimensions", () => {
    it("selects dimension - no modifier, from default setting", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: [],
        clickedDimensionName: "dim1",
        isModifierKeyPressed: false,
      })).toEqual(["dim1"])
    })
    it("unselects - no modifier, when single dimension active", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: ["dim1"],
        clickedDimensionName: "dim1",
        isModifierKeyPressed: false,
      })).toEqual([])
    })
    it("selects - no modifier, when 1 other dimension is active", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: ["dim1"],
        clickedDimensionName: "dim2",
        isModifierKeyPressed: false,
      })).toEqual(["dim2"])
    })
    it("selects - no modifier, when clicked dimension and 1 other is active", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: ["dim1", "dim2"],
        clickedDimensionName: "dim1",
        isModifierKeyPressed: false,
      })).toEqual(["dim1"])
    })

    it("deselects - modifier, all dimensions are active", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: [],
        clickedDimensionName: "dim1",
        isModifierKeyPressed: true,
      })).toEqual(["dim2", "dim3"])
    })
    it("selects - modifier, some dimensions are active", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3", "dim4"],
        selectedDimensions: ["dim1", "dim2"],
        clickedDimensionName: "dim3",
        isModifierKeyPressed: true,
      })).toEqual(["dim1", "dim2", "dim3"])
    })
    it("selects - modifier, allmost all dimensions are active. should return empty array", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: ["dim1", "dim2"],
        clickedDimensionName: "dim3",
        isModifierKeyPressed: true,
      })).toEqual([])
    })
    it("deselects - modifier, only that dimension is active. should return empty array", () => {
      expect(getNewSelectedDimensions({
        allDimensions: ["dim1", "dim2", "dim3"],
        selectedDimensions: ["dim1"],
        clickedDimensionName: "dim1",
        isModifierKeyPressed: true,
      })).toEqual([])
    })
  })
})
