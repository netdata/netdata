import { configure } from "enzyme"
import Adapter from "enzyme-adapter-react-16"

configure({ adapter: new Adapter() })

const NETDATA: any = {
  options: {
    current: {
      timezone: "default",
    },
  },
}

// @ts-ignore
global.NETDATA = NETDATA
