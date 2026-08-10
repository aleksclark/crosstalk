import type { ImgHTMLAttributes } from "react";

export interface LogoProps extends Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt"> {
  alt?: string;
}

export function Logo({ alt = "CrossTalk", ...props }: LogoProps) {
  const appPath = window.location.pathname.split("/")[1];
  const src = appPath ? `/${appPath}/logo.svg` : "/logo.svg";
  return <img src={src} alt={alt} {...props} />;
}
