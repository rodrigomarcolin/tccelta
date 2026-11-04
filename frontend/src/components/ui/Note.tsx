import type { ComponentPropsWithoutRef } from "react";

type Props = ComponentPropsWithoutRef<"p"> & { as?: "p" | "figcaption" };

/** Short secondary text under a block. */
export function Note({ as: Tag = "p", className, ...rest }: Props) {
  const classes = "m-0 mt-2 text-[13px] text-secondary";
  return <Tag className={className ? `${classes} ${className}` : classes} {...rest} />;
}
