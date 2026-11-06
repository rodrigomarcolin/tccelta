import type { ComponentPropsWithoutRef, ElementType } from "react";

type Props<T extends ElementType> = {
  as?: T;
  /** More padding, for whole-screen states. */
  roomy?: boolean;
  /** Do not split the card between printed pages. */
  avoidBreak?: boolean;
  className?: string;
} & Omit<ComponentPropsWithoutRef<T>, "as" | "className">;

/** Surface with a hairline border. Extra classes must not change its box. */
export function Card<T extends ElementType = "div">({
  as,
  roomy,
  avoidBreak,
  className,
  ...rest
}: Props<T>) {
  const Tag: ElementType = as ?? "div";
  const classes = [
    "rounded-lg border border-hairline bg-card",
    roomy ? "p-9" : "p-5",
    avoidBreak ? "print:break-inside-avoid" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");
  return <Tag className={classes} {...rest} />;
}
