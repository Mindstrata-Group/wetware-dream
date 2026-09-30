'use client'

function Avatar({ email }: { email?: string }) {
  const letter = email ? email[0].toUpperCase() : "?";
  return (
    <div style={{
      width: 72,
      height: 72,
      borderRadius: "50%",
      background: "var(--soft)",
      color: "var(--accent-strong)",
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      fontFamily: "var(--font-unbounded, sans-serif)",
      fontWeight: 700,
      fontSize: 28,
      flexShrink: 0,
      border: "2px solid var(--line)",
    }}>
      {letter}
    </div>
  );
}

export { Avatar }
