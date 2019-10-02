import { ChartDetails } from "../chart-types"

export const chartDetailsMock: ChartDetails = {
  id: "system.cpu",
  name: "system.cpu",
  type: "system",
  family: "cpu",
  context: "system.cpu",
  title: "Total CPU utilization (system.cpu)",
  priority: 100,
  plugin: "macos",
  module: "mach_smi",
  enabled: true,
  units: "percentage",
  data_url: "/api/v1/data?chart=system.cpu",
  chart_type: "stacked",
  duration: 3997,
  first_entry: 1569923278,
  last_entry: 1569927274,
  update_every: 1,
  dimensions: {
    user: {
      name: "user",
    },
    nice: {
      name: "nice",
    },
    system: {
      name: "system",
    },
  },
  chart_variables: {},
  green: null,
  red: null,
  alarms: {
    dev_dim_template_system: {
      id: 1569836726,
      status: "CLEAR",
      units: "%",
      update_every: 1,
    },
  },
  url: "/api/v1/chart?chart=system.cpu",
}
