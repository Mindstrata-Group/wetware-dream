"use client";

import { useState } from "react";

// This used to be <input list="..."> + <datalist>: the native browser
// autocomplete is drawn by the OS and looks off/unpredictable (especially outside
// desktop Chrome/Windows). A plain <select> with an explicit "enter
// manually" item looks predictable everywhere and keeps manual
// input possible.
export function ModelPicker({
  value,
  onChange,
  options,
  placeholder,
  className,
  ariaLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  options: string[];
  placeholder?: string;
  className?: string;
  ariaLabel?: string;
}) {
  const [manualOverride, setManualOverride] = useState(false);
  const showSelect = options.length > 0 && !manualOverride;
  const isKnownValue = options.includes(value);
  // A value not among the options is still rendered as a separate <option> below;
  // we switch to __manual__ (the "enter manually" item) only if there is
  // no real value at all, otherwise the select value would diverge from
  // what is actually rendered and the browser would silently show "enter manually".
  const selectValue = value.trim() === "" ? "__manual__" : value;

  if (showSelect) {
    return (
      <select
        className={className}
        aria-label={ariaLabel}
        value={selectValue}
        onChange={(e) => {
          if (e.target.value === "__manual__") {
            setManualOverride(true);
            return;
          }
          onChange(e.target.value);
        }}
        style={{ fontFamily: "monospace", fontSize: 12 }}
      >
        {!isKnownValue && value.trim() !== "" && (
          <option value={value}>{value} (текущая, не из списка)</option>
        )}
        {options.map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
        <option value="__manual__">— указать вручную —</option>
      </select>
    );
  }

  return (
    <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
      <input
        className={className}
        aria-label={ariaLabel}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={{ fontFamily: "monospace", fontSize: 12, flex: 1 }}
      />
      {options.length > 0 && (
        <button
          type="button"
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={() => setManualOverride(false)}
        >
          список
        </button>
      )}
    </div>
  );
}
