export const seconds4human = (
  totalSeconds: number | string, overrideOptions?: {[key: string]: string},
) => {
  const defaultOptions: {[key: string]: string} = {
    now: "now",
    space: " ",
    negative_suffix: "ago",
    day: "day",
    days: "days",
    hour: "hour",
    hours: "hours",
    minute: "min",
    minutes: "mins",
    second: "sec",
    seconds: "secs",
    and: "and",
  }

  const options = typeof overrideOptions === "object"
    ? { ...defaultOptions, ...overrideOptions }
    : defaultOptions

  let seconds = typeof totalSeconds === "string"
    ? parseInt(totalSeconds, 10)
    : totalSeconds

  if (seconds === 0) {
    return options.now
  }

  let suffix = ""
  if (seconds < 0) {
    seconds = -seconds
    if (options.negative_suffix !== "") {
      suffix = options.space + options.negative_suffix
    }
  }

  const days = Math.floor(seconds / 86400)
  seconds -= (days * 86400)

  const hours = Math.floor(seconds / 3600)
  seconds -= (hours * 3600)

  const minutes = Math.floor(seconds / 60)
  seconds -= (minutes * 60)

  const strings = []

  if (days > 1) {
    strings.push(days.toString() + options.space + options.days)
  } else if (days === 1) {
    strings.push(days.toString() + options.space + options.day)
  }

  if (hours > 1) {
    strings.push(hours.toString() + options.space + options.hours)
  } else if (hours === 1) {
    strings.push(hours.toString() + options.space + options.hour)
  }

  if (minutes > 1) {
    strings.push(minutes.toString() + options.space + options.minutes)
  } else if (minutes === 1) {
    strings.push(minutes.toString() + options.space + options.minute)
  }

  if (seconds > 1) {
    strings.push(Math.floor(seconds).toString() + options.space + options.seconds)
  } else if (seconds === 1) {
    strings.push(Math.floor(seconds).toString() + options.space + options.second)
  }

  if (strings.length === 1) {
    return strings.pop() + suffix
  }

  const last = strings.pop()
  return `${strings.join(", ")} ${options.and} ${last}${suffix}`
}
