import type { SVGProps } from "react";

/**
 * Design-system icons: outline, 1.6 to 2 stroke, round caps and joins,
 * `currentColor`. The theme icons (sun and moon) follow the same style.
 */
type Props = SVGProps<SVGSVGElement> & { size?: number };

function Base({ size = 20, children, viewBox = "0 0 20 20", ...rest }: Props) {
  return (
    <svg
      width={size}
      height={size}
      viewBox={viewBox}
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...rest}
    >
      {children}
    </svg>
  );
}

export const IconLocation = (p: Props) => (
  <Base viewBox="0 0 18 18" strokeWidth={1.6} {...p}>
    <path d="M9 2c2.8 0 5 2.1 5 4.8 0 3.3-5 8.2-5 8.2S4 10.1 4 6.8C4 4.1 6.2 2 9 2z" />
    <circle cx="9" cy="6.6" r="1.6" />
  </Base>
);

export const IconReload = (p: Props) => (
  <Base viewBox="0 0 18 18" strokeWidth={2.2} {...p}>
    <path d="M14 3v4h-4" />
    <path d="M14 7A6 6 0 1 0 15 11" />
  </Base>
);

export const IconInfo = (p: Props) => (
  <Base strokeWidth={1.8} {...p}>
    <circle cx="10" cy="10" r="8" />
    <line x1="10" y1="6" x2="10" y2="10.5" />
    <circle cx="10" cy="13.5" r="1" fill="currentColor" stroke="none" />
  </Base>
);

export const IconSignal = (p: Props) => (
  <Base strokeWidth={1.8} {...p}>
    <circle cx="10" cy="10" r="2.5" />
    <circle cx="10" cy="10" r="6" opacity={0.5} />
  </Base>
);

export const IconCheck = (p: Props) => (
  <Base viewBox="0 0 14 14" strokeWidth={2.4} {...p}>
    <path d="M3 7.5l2.5 2.5L11 4" />
  </Base>
);

export const IconChevron = (p: Props) => (
  <Base viewBox="0 0 18 18" strokeWidth={2} {...p}>
    <path d="M6 4l5 5-5 5" />
  </Base>
);

export const IconDongle = (p: Props) => (
  <Base viewBox="0 0 22 22" {...p}>
    <rect x="3" y="5" width="16" height="12" rx="3" />
    <line x1="11" y1="5" x2="11" y2="17" strokeWidth={1.4} />
  </Base>
);

export const IconBack = (p: Props) => (
  <Base viewBox="0 0 22 22" strokeWidth={2} {...p}>
    <path d="M13 5l-6 6 6 6" />
  </Base>
);

export const IconSun = (p: Props) => (
  <Base {...p}>
    <circle cx="10" cy="10" r="3.4" />
    <path d="M10 2.2v1.8M10 16v1.8M2.2 10H4M16 10h1.8M4.5 4.5l1.3 1.3M14.2 14.2l1.3 1.3M15.5 4.5l-1.3 1.3M5.8 14.2l-1.3 1.3" />
  </Base>
);

export const IconMoon = (p: Props) => (
  <Base {...p}>
    <path d="M16.5 11.8A6.8 6.8 0 0 1 8.2 3.5a6.8 6.8 0 1 0 8.3 8.3z" />
  </Base>
);

export const IconTrash = (p: Props) => (
  <Base viewBox="0 0 18 18" strokeWidth={1.6} {...p}>
    <path d="M2.5 4.5h13" />
    <path d="M6.5 4.5V3a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1v1.5" />
    <path d="M4 4.5l.7 9.6a1.5 1.5 0 0 0 1.5 1.4h5.6a1.5 1.5 0 0 0 1.5-1.4l.7-9.6" />
    <path d="M7.5 8v4.5M10.5 8v4.5" />
  </Base>
);
