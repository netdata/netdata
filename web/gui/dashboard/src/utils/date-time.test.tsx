/* eslint-disable global-require */
import { isProperTimezone, useDateTime } from "./date-time"
import { testHook } from "./test-utils"

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

let dateTime: {
  localeDateString: any,
  localeTimeString: any,
  xAxisTimeString: any,
}

beforeEach(() => {
  testHook(() => {
    dateTime = useDateTime()
  })
})

describe("useDateTime", () => {
  it("returns 3 formatters", () => {
    const {
      localeDateString, localeTimeString, xAxisTimeString,
    } = dateTime
    expect(typeof localeDateString).toBe("function")
    expect(typeof localeTimeString).toBe("function")
    expect(typeof xAxisTimeString).toBe("function")

    const date = new Date()
    expect(typeof localeDateString(date)).toBe("string")
    expect(typeof localeTimeString(date)).toBe("string")
    expect(typeof xAxisTimeString(date)).toBe("string")
  })

  it("formats dates", () => {
    const {
      localeDateString, localeTimeString, xAxisTimeString,
    } = dateTime
    expect(typeof localeDateString).toBe("function")
    expect(typeof localeTimeString).toBe("function")
    expect(typeof xAxisTimeString).toBe("function")

    const date = new Date("Aug 13, 2018, 12:34:56")
    expect(localeDateString(date)).toBe("Mon, Aug 13, 2018")
    expect(localeTimeString(date)).toBe("12:34:56")
    expect(xAxisTimeString(date)).toBe("12:34:56")
  })
})
