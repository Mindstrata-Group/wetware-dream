'use client'

import { useEffect, useState } from 'react'
import { absolutePromoAccessUrl, promoAccessUrl } from '../_utils'
import { FONT_MONO, MS } from '../_theme'

function QRCard({ promo }: { promo: string }) {
  const [loaded, setLoaded] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [href, setHref] = useState(() => promoAccessUrl(promo));
  const qrSrc = `https://quickchart.io/qr?size=360&margin=2&text=${encodeURIComponent(href)}`;

  useEffect(() => {
    setHref(absolutePromoAccessUrl(promo));
    setLoaded(false);
    setExpanded(false);
  }, [promo]);

  return (
    <div
      style={{
        background: MS.surface,
        border: `1px solid ${MS.ink10}`,
        borderRadius: 14,
        padding: "18px 20px",
      }}
    >
      <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 10, color: MS.ink }}>
        QR-код активации
      </div>
      <button
        type="button"
        aria-label="Увеличить QR-код активации"
        onClick={() => setExpanded(true)}
        style={{
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
          padding: 16,
          background: MS.surfaceSoft,
          borderRadius: 10,
          border: `1px solid ${MS.ink05}`,
          minHeight: 200,
          width: "100%",
          cursor: promo ? "zoom-in" : "default",
          fontFamily: "inherit",
        }}
      >
        {!loaded && <span style={{ fontSize: 12, color: MS.ink50 }}>QR генерируется…</span>}
        {promo ? (
          <img
            alt={`QR ${promo}`}
            src={qrSrc}
            onLoad={() => setLoaded(true)}
            onError={() => setLoaded(true)}
            style={{ display: loaded ? "block" : "none", width: 180, height: 180 }}
          />
        ) : null}
      </button>
      <div
        style={{
          fontSize: 11,
          color: MS.ink50,
          textAlign: "center",
          marginTop: 10,
          fontFamily: FONT_MONO,
          wordBreak: "break-all",
        }}
      >
        {href}
      </div>
      <a
        href={href}
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: 36,
          marginTop: 10,
          borderRadius: 8,
          border: `1px solid ${MS.ink20}`,
          background: MS.surfaceSoft,
          color: MS.ink70,
          fontSize: 12,
          fontWeight: 500,
          textDecoration: "none",
        }}
      >
        Открыть ссылку промокода
      </a>
      {expanded && (
        <button
          type="button"
          role="dialog"
          aria-modal="true"
          aria-label="Закрыть QR-код активации"
          onClick={() => setExpanded(false)}
          style={{
            position: "fixed",
            inset: 0,
            zIndex: 1000,
            border: 0,
            padding: 24,
            background: "rgba(13,27,22,0.82)",
            cursor: "zoom-out",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexDirection: "column",
            gap: 14,
            fontFamily: "inherit",
          }}
        >
          <span
            style={{
              background: MS.surface,
              borderRadius: 22,
              padding: 18,
              boxShadow: "0 30px 90px rgba(0,0,0,0.35)",
            }}
          >
            <img
              alt={`QR ${promo} крупно`}
              src={qrSrc}
              style={{
                display: "block",
                width: "min(78vw, 520px)",
                height: "min(78vw, 520px)",
              }}
            />
          </span>
          <span style={{ color: "white", fontSize: 13, opacity: 0.9 }}>
            Нажмите ещё раз, чтобы закрыть QR-код
          </span>
        </button>
      )}
    </div>
  );
}

export { QRCard }
