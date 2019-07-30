import { mapObjIndexed } from "ramda"

type OutputValue = string | boolean | number | null | undefined | any[]
// almost the same as in old dashboard to ensure readers that it works the same way
const getDataAttribute = (element: Element, key: string, defaultValue?: OutputValue) => {
  if (element.hasAttribute(key)) {
    // we know it's not null because of hasAttribute()
    const data = element.getAttribute(key) as string

    if (data === "true") {
      return true
    }
    if (data === "false") {
      return false
    }
    if (data === "null") {
      return null
    }

    // Only convert to a number if it doesn't change the string
    if (data === `${+data}`) {
      return +data
    }

    if (/^(?:\{[\w\W]*\}|\[[\w\W]*\])$/.test(data)) {
      return JSON.parse(data)
    }

    return data
  }
  // if no default is passed, then it's undefined and can be replaced with default value later
  // it is recommended to do it in props destructuring assignment phase, ie.:
  // const Chart = ({ dygraphPointsize = 1 }) => ....
  return defaultValue
}

const getDataAttributeBoolean = (element: Element, key: string, defaultValue?: boolean) => {
  const value = getDataAttribute(element, key, defaultValue)

  if (value === true || value === false) { // gmosx: Love this :)
    return value
  }

  if (typeof (value) === "string") {
    if (value === "yes" || value === "on") {
      return true
    }

    if (value === "" || value === "no" || value === "off" || value === "null") {
      return false
    }

    return defaultValue
  }

  if (typeof (value) === "number") {
    return value !== 0
  }

  return defaultValue
}

interface BaseAttributeConfig {
  key: string
  defaultValue?: OutputValue
}
interface BooleanAttributeConfig extends BaseAttributeConfig {
  type: "boolean"
  defaultValue?: boolean
}
type AttributeConfig = BaseAttributeConfig | BooleanAttributeConfig

export type AttributePropKeys = "id" | "host" | "title" | "chartLibrary" | "width" | "height" | "after"
  | "before" | "legend" | "dygraphValueRange" | "dygraphTheme"
type AttributesMap = {
  [key in AttributePropKeys]: AttributeConfig
}

// needs to be a getter so all window.NETDATA settings are set
const getAttributesMap = (): AttributesMap => ({
  id: { key: "data-netdata-react" },
  host: { key: "data-host", defaultValue: window.NETDATA.serverDefault },
  title: { key: "data-title", defaultValue: null },
  chartLibrary: { key: "data-chart-library", defaultValue: window.NETDATA.chartDefaults.library },
  width: { key: "data-width", defaultValue: window.NETDATA.chartDefaults.width },
  height: { key: "data-height", defaultValue: window.NETDATA.chartDefaults.height },
  after: { key: "data-after", defaultValue: window.NETDATA.chartDefaults.after },
  before: { key: "data-after", defaultValue: window.NETDATA.chartDefaults.before },
  legend: { key: "data-legend", type: "boolean", defaultValue: true },
  dygraphValueRange: { key: "data-dygraph-valuerange", defaultValue: [null, null] },
  dygraphTheme: { key: "data-dygraph-theme", defaultValue: "default" },
})

export type Attributes = {
  [key in AttributePropKeys]: OutputValue
}
export const getAttributes = (node: Element): Attributes => mapObjIndexed(
  (attribute: AttributeConfig) => (
    (attribute as BooleanAttributeConfig).type === "boolean"
      ? getDataAttributeBoolean(
        node,
        attribute.key,
          attribute.defaultValue as BooleanAttributeConfig["defaultValue"],
      ) : getDataAttribute(node, attribute.key, attribute.defaultValue)
  ),
  getAttributesMap(),
) as Attributes // need to override because of broken Ramda typings
