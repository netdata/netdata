import { useEffect, useState } from "react"
import { useSelector } from "react-redux"
import { useInterval } from "react-use"

import { selectHasWindowFocus } from "domains/global/selectors"
import { BIGGEST_INTERVAL_NUMBER } from "utils/biggest-interval-number"


type UseFetchNewDataClock = (arg: {
  areCriteriaMet: boolean
  preferedIntervalTime: number
}) => [boolean, (shouldFetch: boolean) => void]
export const useFetchNewDataClock: UseFetchNewDataClock = ({
  areCriteriaMet,
  preferedIntervalTime,
}) => {
  const hasWindowFocus = useSelector(selectHasWindowFocus)
  const [shouldFetch, setShouldFetch] = useState<boolean>(true)
  const [shouldFetchImmediatelyAfterFocus, setShouldFetchImmediatelyAfterFocus] = useState(false)

  useEffect(() => {
    if (shouldFetchImmediatelyAfterFocus && hasWindowFocus) {
      setShouldFetchImmediatelyAfterFocus(false)
      setShouldFetch(true)
    }
  }, [shouldFetchImmediatelyAfterFocus, setShouldFetchImmediatelyAfterFocus, hasWindowFocus])

  // don't use setInterval when we loose focus
  const intervalTime = (hasWindowFocus || !shouldFetchImmediatelyAfterFocus)
    ? preferedIntervalTime
    : BIGGEST_INTERVAL_NUMBER
  useInterval(() => {
    if (areCriteriaMet) {
      if (!hasWindowFocus) {
        setShouldFetchImmediatelyAfterFocus(true)
        return
      }
      setShouldFetch(true)
    }
    // when there's no focus, don't ask for updated data
  }, intervalTime)
  return [shouldFetch, setShouldFetch]
}
