type ColorHex2Rgb = (hex: string) => {
  r: number,
  g: number,
  b: number
}
export const colorHex2Rgb: ColorHex2Rgb = (hex) => {
  // Expand shorthand form (e.g. "03F") to full form (e.g. "0033FF")
  const shorthandRegex = /^#?([a-f\d])([a-f\d])([a-f\d])$/i
  const hexFull = hex.replace(shorthandRegex, (m, r, g, b) => r + r + g + g + b + b)

  const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hexFull)
  if (!result) {
    console.warn("wrong color format:", hex) // eslint-disable-line no-console
  }
  return result
    ? {
      r: parseInt(result[1], 16),
      g: parseInt(result[2], 16),
      b: parseInt(result[3], 16),
    } : {
      r: 255,
      g: 0,
      b: 0,
    }
}
