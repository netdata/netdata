export interface ChartData {
}

interface Dimension {
  name: string
}

export interface ChartDetails {
  alarms: {}
  /* eslint-disable camelcase */
  chart_type: string
  context?: string
  data_url: string
  dimensions: {
    [key: string]: Dimension
  }
  duration: number
  enabled: boolean
  family: string
  first_entry: number
  // green: ?
  id: string
  last_entry: number
  module?: string
  name: string
  plugin?: string
  priority: number
  // red: ?
  title: string
  type: string
  units: string
  update_every: number
}

export interface ChartState {
  chartData: ChartData | null
  chartDetails: ChartDetails | null
}
