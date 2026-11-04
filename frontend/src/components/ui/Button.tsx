import type { ButtonHTMLAttributes, ReactNode } from "react";

type Variant = "primary" | "secondary" | "tonal" | "link";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  icon?: ReactNode;
  fullWidth?: boolean;
};

/** Semantic button: the variant carries meaning, not just emphasis. */
export function Button({
  variant = "primary",
  icon,
  fullWidth,
  className,
  children,
  type = "button",
  ...rest
}: Props) {
  const classes = ["button", `button-${variant}`, fullWidth ? "button-block" : "", className ?? ""]
    .filter(Boolean)
    .join(" ");
  return (
    <button type={type} className={classes} {...rest}>
      {icon}
      {children}
    </button>
  );
}
