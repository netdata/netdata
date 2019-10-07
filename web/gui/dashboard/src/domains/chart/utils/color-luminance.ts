import {
  cond, identity, map, pipe, replace, splitEvery, T, toString,
} from "ramda"

type NormalizeHex = (hex: string) => string
export const normalizeHex: NormalizeHex = pipe(
  toString,
  replace(/[^0-9a-f]/gi, ""),
  cond([
    [(str) => str.length < 6, (str) => str[0] + str[0] + str[1] + str[1] + str[2] + str[2]],
    [T, identity],
  ]),
)

export const colorLuminance = (hex: string, lum: number = 0) => {
  const hexNormalized = normalizeHex(hex)

  // convert to decimal and change luminosity
  const rgb = pipe(
    // point-free version generates ts error
    (str: string) => splitEvery(2, str),
    map(
      pipe(
        (str: string) => parseInt(str, 16),
        (nr) => Math.round(
          Math.min(
            Math.max(0, nr + (nr * lum)),
            255,
          ),
        ).toString(16),
        (str) => `00${str}`.substr(str.length),
      ),
    ),
    (x) => x.join(""),
  )(hexNormalized)
  return `#${rgb}`
}
