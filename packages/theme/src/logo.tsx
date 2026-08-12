import type { ImgHTMLAttributes } from "react";

export interface LogoProps extends Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt"> {
  alt?: string;
}

/**
 * Product logo. Resolves under each SPA base path (/admin, /translator, /broadcast).
 */
export function Logo({ alt = "CrossTalk", style, ...props }: LogoProps) {
  const appPath = typeof window !== "undefined" ? window.location.pathname.split("/")[1] : "";
  const src = appPath ? `/${appPath}/logo.svg` : "/logo.svg";
  return (
    <img
      src={src}
      alt={alt}
      style={{ display: "block", ...style }}
      {...props}
    />
  );
}
