// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import type { AnchorHTMLAttributes, ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger" | "warning" | "success";

const BASE =
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-full px-4 py-2 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const VARIANT_CLASS: Record<Variant, string> = {
  primary: "bg-accent text-on-accent shadow-card hover:bg-accent-hover",
  secondary: "bg-surface-1 text-text-primary shadow-card hover:bg-surface-2",
  danger: "bg-surface-1 text-status-critical shadow-card hover:bg-surface-2",
  warning: "bg-surface-1 text-status-warning shadow-card hover:bg-surface-2",
  success: "bg-surface-1 text-status-good shadow-card hover:bg-surface-2",
};

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return <button className={`${BASE} ${VARIANT_CLASS[variant]} ${className}`} {...props} />;
}

export function LinkButton({
  variant = "primary",
  className = "",
  href,
  ...props
}: AnchorHTMLAttributes<HTMLAnchorElement> & { variant?: Variant; href: string }) {
  return <Link href={href} className={`${BASE} ${VARIANT_CLASS[variant]} ${className}`} {...props} />;
}
