import { useMemo } from "react"
import { useSelector } from "react-redux"

import { selectTimezone } from "domains/global/selectors"

const zeropad = (x: number) => {
  if (x > -10 && x < 10) {
    return `0${x.toString()}`
  }
  return x.toString()
}

export const isSupportingDateTimeFormat = !!(Intl && Intl.DateTimeFormat)

const narrowToDate = (d: Date | number) => (typeof d === "number"
  ? new Date(d)
  : d
)
// these are the old netdata functions
// we fallback to these, if the new ones fail
export const localeDateStringNative = (d: Date | number) => narrowToDate(d).toLocaleDateString()
export const localeTimeStringNative = (d: Date | number) => narrowToDate(d).toLocaleTimeString()
export const xAxisTimeStringNative = (d: Date | number) => {
  const date = narrowToDate(d)
  return `${zeropad(date.getHours())}:${
    zeropad(date.getMinutes())}:${
    zeropad(date.getSeconds())}`
}


export const isProperTimezone = (timeZone: string): boolean => {
  try {
    Intl.DateTimeFormat(navigator.language, {
      localeMatcher: "best fit",
      formatMatcher: "best fit",
      weekday: "short",
      year: "numeric",
      month: "short",
      day: "2-digit",
      timeZone,
    })
  } catch (e) {
    return false
  }
  return true
}

export const useDateTime = () => {
  const timezone = useSelector(selectTimezone)
  const isUsingTimezone = typeof timezone === "string" && timezone !== "" && timezone !== "default"

  const localeDateString = useMemo(() => {
    const dateOptions = {
      localeMatcher: "best fit",
      formatMatcher: "best fit",
      weekday: "short",
      year: "numeric",
      month: "short",
      day: "2-digit",
      timeZone: isUsingTimezone ? timezone : undefined,
    }
    const dateFormat = () => new Intl.DateTimeFormat(navigator.language, dateOptions)
    return isSupportingDateTimeFormat
      ? (d: Date | number) => dateFormat().format(d)
      : localeDateStringNative
  }, [timezone, isUsingTimezone])


  const localeTimeString = useMemo(() => {
    const timeOptions = {
      localeMatcher: "best fit",
      hour12: false,
      formatMatcher: "best fit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      timeZone: isUsingTimezone ? timezone : undefined,
      timeZoneName: isUsingTimezone ? "short" : undefined,
    }
    const timeFormat = () => new Intl.DateTimeFormat(navigator.language, timeOptions)
    return isSupportingDateTimeFormat
      ? (d: Date | number) => timeFormat().format(d)
      : localeTimeStringNative
  }, [timezone, isUsingTimezone])


  const xAxisTimeString = useMemo(() => {
    const xAxisOptions = {
      localeMatcher: "best fit",
      hour12: false,
      formatMatcher: "best fit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      timeZone: isUsingTimezone ? timezone : undefined,
    }
    const xAxisFormat = () => new Intl.DateTimeFormat(navigator.language, xAxisOptions)
    return isSupportingDateTimeFormat
      ? (d: Date | number) => xAxisFormat().format(d)
      : xAxisTimeStringNative
  }, [timezone, isUsingTimezone])

  return {
    localeDateString,
    localeTimeString,
    xAxisTimeString,
  }
}
