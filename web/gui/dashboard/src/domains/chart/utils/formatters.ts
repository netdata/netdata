import { useEffect, useState } from "react"
import { unitsConversionCreator } from "utils/units-conversion"

type Converter = (v: number) => number | string
// only time units are converted into strings, the rest are numbers

interface Attributes {
  units: string,
  unitsCommon: string,
  unitsDesired: string,
  uuid: string,
}
export const useFormatters = ({
  units,
  unitsCommon,
  unitsDesired,
  uuid,
}: Attributes) => {
  // previously _unitsConversion
  const [convertUnits, setConvertUnits] = useState<Converter>()

  // probably can also be removed
  const [min, setMin] = useState<number>()
  const [max, setMax] = useState<number>()

  const [unitsCurrent, setUnitsCurrent] = useState<string>(units)

  // unitConversionSetup
  useEffect(() => {
    if (typeof (min) === "number" && typeof max === "number") {
      setConvertUnits(
        () => unitsConversionCreator.get(uuid, min, max, units, unitsDesired, unitsCommon,
          (switchedUnits) => {
            setUnitsCurrent(switchedUnits)
            // that.legendSetUnitsString(that.units_current);
            // that.legendSetUnitsString just populates some DOM with unitsCurrent
            // on all occurences just take the unitsCurrent from this state
          }),
      )
    }
  }, [units, unitsDesired, min, max, uuid, unitsCommon])

  const legendFormatValueDecimalsFromMinMax = (newMin: number, newMax: number) => {
    if (newMin === min && newMax === max) {
      return
    }
    setMin(newMin)
    setMax(newMax)
  }

  return {
    legendFormatValueDecimalsFromMinMax,
    convertUnits,
    unitsCurrent,
  }
}
