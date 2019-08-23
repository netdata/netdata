import { identity } from "ramda"
import { useCallback, useState, useMemo } from "react"
import { unitsConversionCreator } from "utils/units-conversion"
import { ChartData } from "../chart-types"
import { Attributes } from "./transformDataAttributes"

type Converter = (v: number) => number | string
// only time units are converted into strings, the rest are numbers

const getLegendFormatValue = (
  convertUnits: Converter, intlNumberFormat: Intl.NumberFormat | null, valueDecimalDetail: number,
) => (value: number | string) => {
  if (typeof value !== "number") {
    return "-"
  }

  const convertedValue = convertUnits(value)
  if (typeof convertedValue !== "number") {
    return convertedValue
  }

  if (intlNumberFormat !== null) {
    return intlNumberFormat.format(convertedValue)
  }

  let dmin
  let dmax
  if (valueDecimalDetail !== -1) {
    dmin = valueDecimalDetail
    dmax = valueDecimalDetail
  } else {
    dmin = 0
    const abs = (value < 0) ? -value : value
    if (abs > 1000) {
      dmax = 0
    } else if (abs > 10) {
      dmax = 1
    } else if (abs > 1) {
      dmax = 2
    } else if (abs > 0.1) {
      dmax = 2
    } else if (abs > 0.01) {
      dmax = 4
    } else if (abs > 0.001) {
      dmax = 5
    } else if (abs > 0.0001) {
      dmax = 6
    } else {
      dmax = 7
    }
  }

  return window.NETDATA.fastNumberFormat.get(dmin, dmax).format(value)
}

interface Arguments {
  attributes: Attributes,
  data: ChartData,
  units: string,
  unitsCommon: string | undefined,
  unitsDesired: string,
  uuid: string,
}
export const useFormatters = ({
  attributes,
  data,
  units,
  unitsCommon,
  unitsDesired,
  uuid,
}: Arguments) => {
  // previously _unitsConversion
  const [convertUnits, setConvertUnits] = useState<Converter>(() => identity)

  type LegendFormatValue = (value: string | number) => string | number

  // probably can also be removed
  const [min, setMin] = useState<number>()
  const [max, setMax] = useState<number>()

  // todo most of this state is not needed, that hook can be refractored
  const [unitsCurrent, setUnitsCurrent] = useState<string>(units)

  const [decimals, setDecimals] = useState<number>(-1)
  const [intlNumberFormat, setIntlNumberFormat] = useState<Intl.NumberFormat | null>(null)

  const {
    // "valueDecimalDetail" in old app
    decimalDigits = -1,
  } = attributes

  const legendFormatValueDecimalsFromMinMax = useCallback((newMin: number, newMax: number) => {
    if (newMin === min && newMax === max) {
      return
    }
    // we should call the convertUnits-creation only when original app was doing this
    // so we don't get new updates in improper places
    setMin(newMin)
    setMax(newMax)

    const newConvertUnits = unitsConversionCreator.get(
      uuid, newMin, newMax, units, unitsDesired, unitsCommon,
      (switchedUnits) => {
        setUnitsCurrent(switchedUnits)
        // that.legendSetUnitsString(that.units_current);
        // that.legendSetUnitsString just populates some DOM with unitsCurrent
        // on all occurences just take the unitsCurrent from this state
      },
    )

    // as function, so useState() interpretes it properly
    setConvertUnits(() => newConvertUnits)

    const convertedMin = newConvertUnits(newMin)
    const convertedMax = newConvertUnits(newMax)

    if (typeof convertedMin !== "number" || typeof convertedMax !== "number") {
      return
    }

    let newDecimals = decimals

    if (data.min === data.max) {
      // it is a fixed number, let the visualizer decide based on the value
      newDecimals = -1
    } else if (decimalDigits !== -1) {
      // there is an override
      newDecimals = decimalDigits
    } else {
      // ok, let's calculate the proper number of decimal points
      let delta

      if (convertedMin === convertedMax) {
        delta = Math.abs(convertedMin)
      } else {
        delta = Math.abs(convertedMax - convertedMin)
      }

      if (delta > 1000) {
        newDecimals = 0
      } else if (delta > 10) {
        newDecimals = (1)
      } else if (delta > 1) {
        newDecimals = 2
      } else if (delta > 0.1) {
        newDecimals = 2
      } else if (delta > 0.01) {
        newDecimals = 4
      } else if (delta > 0.001) {
        newDecimals = 5
      } else if (delta > 0.0001) {
        newDecimals = 6
      } else {
        newDecimals = 7
      }
    }


    let newIntlNumberFormat = intlNumberFormat

    if (newDecimals !== decimals) {
      if (newDecimals < 0) {
        newIntlNumberFormat = null
      } else {
        // todo refractor fastNumberFormat
        newIntlNumberFormat = window.NETDATA.fastNumberFormat.get(
          newDecimals,
          newDecimals,
        )
      }
      setIntlNumberFormat(() => newIntlNumberFormat)
      setDecimals(newDecimals)
    }
  }, [
    decimals, decimalDigits, min, max, uuid, units, unitsDesired, unitsCommon,
    data.min, data.max, intlNumberFormat,
  ])

  const legendFormatValue: LegendFormatValue = useMemo(() => (
    getLegendFormatValue(
      convertUnits, intlNumberFormat, decimalDigits,
    )
  ), [convertUnits, decimalDigits, intlNumberFormat])

  return {
    legendFormatValue,
    legendFormatValueDecimalsFromMinMax,
    unitsCurrent,
  }
}
