import * as React from "react"
import classNames from "classnames"

import "./button.css"

type Props = React.ButtonHTMLAttributes<HTMLButtonElement>
export const Button = React.forwardRef(({
  children,
  className,
  ...rest
}: Props, ref: React.Ref<HTMLButtonElement>) => (
  <button
    {...rest} // eslint-disable-line react/jsx-props-no-spreading
    type="button"
    className={classNames("netdata-reset-button", className)}
    ref={ref}
  >
    {children}
  </button>
))
