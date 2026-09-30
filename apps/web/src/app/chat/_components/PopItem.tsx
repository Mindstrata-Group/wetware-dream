"use client";

import { useState } from "react";
import Link from "next/link";
import { T } from "../theme";

// #8: if href is set, render a <Link> with a real <a> inside so that
// Ctrl/Cmd+Click and middle-click open in a new tab. onClick is still
// supported for the popup menu (closing).
export function PopItem({ children, onClick, href, danger, icon }: {
  children: React.ReactNode;
  onClick?: () => void;
  href?: string;
  danger?: boolean;
  icon?: React.ReactNode;
}) {
  const [h, setH] = useState(false);
  const style: React.CSSProperties = {
    width: "100%", textAlign: "left", padding: "8px 10px", borderRadius: 6, border: "none",
    background: h ? (danger ? "#FBECEC" : T.surfaceSoft) : "transparent",
    color: danger ? T.red : T.ink, cursor: "pointer", fontFamily: T.fontBody, fontSize: 13,
    display: "flex", alignItems: "center", gap: 8, textDecoration: "none",
  };
  const content = (
    <>
      {icon && <span style={{ color: danger ? T.red : T.ink50 }}>{icon}</span>}
      {children}
    </>
  );
  if (href) {
    return (
      <Link href={href} onClick={onClick} onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)} style={style}>
        {content}
      </Link>
    );
  }
  return (
    <button onClick={onClick} onMouseEnter={() => setH(true)} onMouseLeave={() => setH(false)} style={style}>
      {content}
    </button>
  );
}
