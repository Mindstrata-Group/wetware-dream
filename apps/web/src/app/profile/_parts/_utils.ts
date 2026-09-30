export function daysLeft(value?: string | null) {
  if (!value) return "";
  const ms = new Date(value).getTime() - Date.now();
  if (!Number.isFinite(ms)) return "";
  const days = Math.max(0, Math.ceil(ms / 86400000));
  return ` · осталось ${days} дн.`;
}
