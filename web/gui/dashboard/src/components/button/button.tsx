import * as React from "react"
import classNames from "classnames"

import "./button.css"

type Props = React.ButtonHTMLAttributes<HTMLButtonElement>
export const Button = ({
  children,
  className,
  ...rest
}: Props) => {
  return (
    // eslint-disable-next-line react/jsx-props-no-spreading
    <button {...rest} type="button" className={classNames("netdata-reset-button", className)}>
      {children}
    </button>
  )
}
