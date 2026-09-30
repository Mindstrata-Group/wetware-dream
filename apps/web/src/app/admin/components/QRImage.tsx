"use client";

import { useEffect, useState } from "react";
import { fullClientUrl } from "../utils";

export function QRImage({ code, url }: { code: string; url: string }) {
  const [loaded, setLoaded] = useState(false);
  const [qrUrl, setQrUrl] = useState(url);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    setQrUrl(fullClientUrl(url));
  }, [url]);

  return (
    <button
      type="button"
      className={expanded ? "ms-qr-wrap is-expanded" : "ms-qr-wrap"}
      onClick={() => setExpanded((v) => !v)}
      aria-label={expanded ? "Свернуть QR" : "Развернуть QR на весь экран"}
    >
      {!loaded && <span className="ms-spinner" aria-label="QR генерируется" />}
      <img
        className="ms-qr"
        alt={`QR ${code}`}
        src={`https://quickchart.io/qr?size=180&margin=2&text=${encodeURIComponent(qrUrl)}`}
        onLoad={() => setLoaded(true)}
        onError={() => setLoaded(true)}
      />
    </button>
  );
}
