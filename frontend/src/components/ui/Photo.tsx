import { useState } from "react";

// Photo は写真なし・読み込み失敗時にも、同じ領域に代替表示を残す。
export function Photo({ src, alt, className, fallbackClassName, fallback, lazy = true }: {
  src: string | null;
  alt: string;
  className: string;
  fallbackClassName: string;
  fallback: string;
  lazy?: boolean;
}) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  if (!src || src === failedSrc) return <div className={fallbackClassName}>{fallback}</div>;
  return <img src={src} alt={alt} className={className} loading={lazy ? "lazy" : undefined} onError={() => setFailedSrc(src)} />;
}
