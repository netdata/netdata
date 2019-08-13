/* eslint-disable global-require */
import { isProperTimezone } from "./date-time"

jest.mock("react-redux", () => ({
  useSelector: () => "default",
}))
jest.mock("../domains/global/selectors", () => ({
  selectTimezone: () => "mock",
}))

describe("isProperTimezone", () => {
  it("return false on improper timezone", () => {
    expect(isProperTimezone("EEST")).toBe(false)
  })
  it("returns true on proper timezone", () => {
    expect(isProperTimezone("Europe/Athens")).toBe(true)
  })
})
