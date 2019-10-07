import { colorLuminance, normalizeHex } from "../color-luminance"

describe("colorLuminance", () => {
  describe("normalizeHex", () => {
    it("doesnt change proper hex", () => {
      expect(normalizeHex("0055ff")).toEqual("0055ff")
    })
    it("handles 3-digit hex", () => {
      expect(normalizeHex("1fa")).toEqual("11ffaa")
    })
    it("removes invalid characters", () => {
      expect(normalizeHex("0055ff Z")).toEqual("0055ff")
      expect(normalizeHex("05f ")).toEqual("0055ff")
    })
  })

  describe("colorLuminance", () => {
    it("doesnt change hex if luminance is 0", () => {
      expect(colorLuminance("00ff55", 0)).toEqual("#00ff55")
      expect(colorLuminance("1fa", 0)).toEqual("#11ffaa")
      expect(colorLuminance("0055ff ", 0)).toEqual("#0055ff")
    })

    it("changes hex based on luminance", () => {
      expect(colorLuminance("00ff55", 0.5)).toEqual("#00ff80")
      expect(colorLuminance("00ff55", 1)).toEqual("#00ffaa")
      expect(colorLuminance("22ee11", 0.2)).toEqual("#29ff14")
    })
  })
})
