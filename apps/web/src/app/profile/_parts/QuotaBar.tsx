'use client'

function QuotaBar({ used, limit }: { used: number; limit: number }) {
  const pct = limit > 0 ? Math.min(100, (used / limit) * 100) : 0;
  const color = pct > 80 ? "#ef4444" : pct > 50 ? "#f59e0b" : "var(--accent)";
  return (
    <div style={{ height: 6, borderRadius: 3, background: "var(--line)", overflow: "hidden", marginTop: 10 }}>
      <div style={{ width: `${pct}%`, height: "100%", background: color, borderRadius: 3, transition: "width 0.4s ease" }} />
    </div>
  );
}

export { QuotaBar }
