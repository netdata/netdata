import { createAction } from "redux-act"

// slightly simplified version of the creator used in the cloud
// we will unify it when some typing issues will be fixed (cloud version didn't warn on bad payload)
export const createRequestAction = <RequestT, SuccessT = any, FailureT = any>(name: string) => {
  const action = createAction<RequestT>(name.toUpperCase())

  return Object.assign(action, {
    request: action,
    success: createAction<SuccessT>(
      `${name.toUpperCase()}_SUCCESS`,
      (payload) => payload,
      (meta) => meta,
    ),
    failure: createAction<FailureT>(
      `${name.toUpperCase()}_FAILURE`,
      (payload) => payload,
      (meta) => ({
        ...meta,
        error: true,
      }),
    ),
  })
}
