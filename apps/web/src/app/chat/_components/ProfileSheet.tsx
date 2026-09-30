"use client";

import Link from "next/link";
import { T, btnReset } from "../theme";

export function ProfileSheet({ onClose }: { onClose: () => void }) {
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(13,27,22,0.4)", display: "flex", alignItems: "flex-end", zIndex: 60 }}>
      <div onClick={e => e.stopPropagation()} style={{ width: "100%", background: T.surface, borderTopLeftRadius: 18, borderTopRightRadius: 18, padding: "12px 16px 28px" }}>
        <div style={{ height: 4, width: 38, background: T.ink20, borderRadius: 2, margin: "0 auto 14px" }} />
        <Link href="/access?force=promo" style={{ ...btnReset, width: "100%", height: 46, borderRadius: T.radiusSm, background: T.green, color: "#fff", fontSize: 14, fontWeight: 600, marginBottom: 6 }}>Продлить доступ</Link>
        <Link href="/profile" style={{ ...btnReset, width: "100%", height: 44, borderRadius: T.radiusSm, border: `1px solid ${T.ink10}`, color: T.ink, fontSize: 14, marginBottom: 6 }} onClick={onClose}>Профиль</Link>
        <Link href="/privacy" style={{ ...btnReset, width: "100%", height: 44, borderRadius: T.radiusSm, border: `1px solid ${T.ink10}`, color: T.ink70, fontSize: 14, marginBottom: 6 }} onClick={onClose}>Политика конфиденциальности</Link>
        <div style={{ height: 1, background: T.ink10, margin: "4px 0 6px" }} />
        <Link href="/login" style={{ ...btnReset, width: "100%", height: 44, borderRadius: T.radiusSm, border: `1px solid ${T.ink10}`, color: T.red, fontSize: 14 }}>Выйти</Link>
      </div>
    </div>
  );
}
