import type { ButtonHTMLAttributes } from "react";
import { clsx } from "clsx";
export function Button({
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      className={clsx("console-button", className)}
      {...props}
    />
  );
}
