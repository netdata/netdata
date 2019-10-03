import React from "react"
import { Provider } from "react-redux"
import { createStore, Reducer } from "redux"
// eslint-disable-next-line import/no-extraneous-dependencies
import { render } from "@testing-library/react"

export function storeProviderMock<T, RT>(reducerProvider: (arg?: T) => Reducer<RT>) {
  return (ui: JSX.Element, state?: T) => {
    const store = createStore(reducerProvider(state))
    return {
      ...render(
        <Provider store={store}>
          {ui}
        </Provider>,
      ),
      store,
    }
  }
}
