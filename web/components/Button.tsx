import Link from "next/link";
import type { AnchorHTMLAttributes, ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger" | "warning" | "success";

const VARIANT_CLASS: Record<Variant, string> = {
  primary:
    "rounded-full bg-accent px-5 py-2 font-semibold text-on-accent hover:bg-accent-hover disabled:opacity-50",
  secondary:
    "rounded-full border-2 border-border-strong px-4 py-1.5 text-text-primary hover:border-text-secondary disabled:opacity-50",
  danger:
    "rounded-full border-2 border-border-strong px-4 py-1.5 text-status-critical hover:border-status-critical disabled:opacity-50",
  warning:
    "rounded-full border-2 border-border-strong px-4 py-1.5 text-status-warning hover:border-status-warning disabled:opacity-50",
  success:
    "rounded-full border-2 border-border-strong px-4 py-1.5 text-status-good hover:border-status-good disabled:opacity-50",
};

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return (
    <button
      className={`text-sm transition-colors ${VARIANT_CLASS[variant]} ${className}`}
      {...props}
    />
  );
}

export function LinkButton({
  variant = "primary",
  className = "",
  href,
  ...props
}: AnchorHTMLAttributes<HTMLAnchorElement> & { variant?: Variant; href: string }) {
  return (
    <Link
      href={href}
      className={`inline-block text-sm transition-colors ${VARIANT_CLASS[variant]} ${className}`}
      {...props}
    />
  );
}
