/* eslint-disable react/jsx-props-no-spreading */
import React from "react"
import Dygraph from "dygraphs"

import "../../../../../../public/dashboard-react"
import { storeProviderMock } from "test/store-provider-mock"
import { DygraphChart } from "../dygraph-chart"

import { attributesMock } from "../../../__mocks__/attributes-mock"
import { chartDataMock } from "../../../__mocks__/chart-data-mock"
import { chartDetailsMock } from "../../../__mocks__/chart-details"
import { mockStoreWithGlobal } from "../../../__mocks__/store.mock"
import { ChartLibraryName } from "../../../utils/chartLibrariesSettings"

const reduxProvider = storeProviderMock(mockStoreWithGlobal)

// a hack to get access to dygraph instance. we cannot inspect canvas
// so a way to check proper rendering is using dygraph-instance methods like .xAxisRange()
// unfortunately there is no way (yet?) to access hook in enzyme/react-testing-library
const isDygraphState = (state: { [key: string]: any }) => !!state.updateOptions
const mockUseState = () => {
  const useStateOriginal = React.useState
  const useStateSpy = jest.spyOn(React, "useState")
  const stateRef: { current: Dygraph | null } = { current: null }
  // @ts-ignore
  useStateSpy.mockImplementation((init?: Dygraph) => {
    const [origState, setOrigState] = useStateOriginal(init)
    const mockSetter = (newState: Dygraph) => {
      if (isDygraphState(newState)) {
        stateRef.current = newState
      }
      setOrigState(newState)
    }
    return [origState, mockSetter]
  })
  return stateRef
}

const dygraphDefaultProps = {
  attributes: attributesMock,
  chartData: chartDataMock,
  chartDetails: chartDetailsMock,
  chartLibrary: "dygraph" as ChartLibraryName,
  chartUuid: "chart-uuid",
  colors: {},
  dimensionsVisibility: [true, true],
  isRemotelyControlled: true,
  legendFormatValue: (v: number) => v,
  onUpdateChartPanAndZoom: () => {},
  orderedColors: ["#ff00ff", "#00ffff", "#ffff00"],
  hoveredX: null,
  setGlobalChartUnderlay: () => {},
  setHoveredX: () => {},
  setMinMax: () => {},
  unitsCurrent: "units-current",
  viewAfter: chartDataMock.after,
  viewBefore: chartDataMock.before,
}

describe("dygraph-chart", () => {
  const dygraphInstanceState = mockUseState()

  it("should render in proper range", () => {
    reduxProvider(
      <DygraphChart
        {...dygraphDefaultProps}
      />,
    )
    const dygraphInstance = dygraphInstanceState.current as Dygraph
    const range = dygraphInstance.xAxisRange()
    expect(range[0]).toBe(chartDataMock.after)
    expect(range[1]).toBe(chartDataMock.before)
  })
})
