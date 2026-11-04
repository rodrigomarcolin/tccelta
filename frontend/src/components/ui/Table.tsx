import type { ComponentPropsWithoutRef } from "react";
import { DATA_VALUE } from "./DataValue";

export function Table({ className, ...rest }: ComponentPropsWithoutRef<"table">) {
  const classes = "w-full border-collapse text-[13px]";
  return <table className={className ? `${classes} ${className}` : classes} {...rest} />;
}

const CELL = "border-b border-hairline px-3 py-2";
const align = (num?: boolean) => (num ? "text-right" : "text-left");

/** Header cell (`scope="col"`) or row header (`scope="row"`); `num` aligns to the right. */
export function Th({
  num,
  scope,
  className,
  ...rest
}: ComponentPropsWithoutRef<"th"> & { num?: boolean }) {
  const look = scope === "col" ? "type-overline tracking-[0.5px] text-secondary" : "font-normal";
  const classes = [CELL, look, align(num), className ?? ""].filter(Boolean).join(" ");
  return <th scope={scope} className={classes} {...rest} />;
}

export function Td({
  num,
  mono,
  className,
  ...rest
}: ComponentPropsWithoutRef<"td"> & { num?: boolean; mono?: boolean }) {
  const classes = [CELL, "font-normal", align(num), mono ? DATA_VALUE : "", className ?? ""]
    .filter(Boolean)
    .join(" ");
  return <td className={classes} {...rest} />;
}
