"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { BrandLogo } from "@/components/BrandLogo";
import { MessageContent } from "@/components/MessageContent";
import { apiFetch, ApiError } from "@/lib/api";
import type { ProfilePayload } from "./_parts/_types";
import { daysLeft } from "./_parts/_utils";
import { Avatar } from "./_parts/Avatar";
import { QuotaBar } from "./_parts/QuotaBar";
export default function ProfilePage() {
  const router = useRouter();
  const [profile, setProfile] = useState<ProfilePayload | null>(null);
  const [error, setError] = useState("");
  const [theme, setTheme] = useState<"light" | "dark" | "auto">("auto");
  const [headerMobile, setHeaderMobile] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [savingAnonymization, setSavingAnonymization] = useState(false);
  const [savingNotification, setSavingNotification] = useState("");
  const [disablingAutoRenew, setDisablingAutoRenew] = useState(false);
  // Waiting for the Max link: after a click on the toggle we poll the status so the checkmark
  // turns green without a manual refresh.
  const [maxLinking, setMaxLinking] = useState(false);
  const maxPollRef = useRef<number | null>(null);
  const [maxUnlinking, setMaxUnlinking] = useState(false);
  const [maxNotice, setMaxNotice] = useState("");
  const [tgLinking, setTgLinking] = useState(false);
  const [tgUnlinking, setTgUnlinking] = useState(false);
  const tgPollRef = useRef<number | null>(null);
  async function load() {
    setError("");
    try {
      const json = await apiFetch<ProfilePayload>("/api/profile");
      setProfile(json);
    } catch (e) {
      const message = e instanceof Error ? e.message : "Ошибка профиля";
      if (message === "auth required") {
        router.replace("/login");
        return;
      }
      setError(message);
    }
  }
  async function logout() {
    await apiFetch("/api/auth/logout", { method: "POST" }).catch(() => null);
    router.push("/login");
  }
  async function updateMessageAnonymization(allowMessageAnonymization: boolean) {
    if (!profile) return;
    const previous = profile.allowMessageAnonymization;
    setSavingAnonymization(true);
    setError("");
    setProfile({ ...profile, allowMessageAnonymization });
    try {
      const json = await apiFetch<{ ok: boolean; allowMessageAnonymization: boolean }>("/api/profile/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ allowMessageAnonymization }),
      });
      setProfile((current) => current ? { ...current, allowMessageAnonymization: json.allowMessageAnonymization } : current);
    } catch {
      setProfile((current) => current ? { ...current, allowMessageAnonymization: previous } : current);
      setError("Не удалось сохранить настройку анонимизации. Попробуйте ещё раз.");
    } finally {
      setSavingAnonymization(false);
    }
  }
  async function updateNotificationConsent(channel: string, consentType: "service" | "marketing", granted: boolean, reason: string) {
    if (!profile) return;
    const previous = profile;
    const key = `${channel}:${consentType}`;
    setSavingNotification(key);
    setError("");
    try {
      const json = await apiFetch<{ ok: boolean; grantedBonus?: number; notifications?: ProfilePayload["notifications"]; contacts?: NonNullable<ProfilePayload["notifications"]>["contacts"]; consents?: NonNullable<ProfilePayload["notifications"]>["consents"]; reachability?: NonNullable<ProfilePayload["notifications"]>["reachability"] }>("/api/notifications/preferences", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ channel, consentType, granted, reason }),
      });
      setProfile((current) => current ? { ...current, notifications: { ...current.notifications, contacts: json.contacts, consents: json.consents, reachability: json.reachability } } : current);
      if (granted && (json.grantedBonus ?? 0) > 0) {
        setMaxNotice(`Подписка включена — начислили +${json.grantedBonus} сообщений к дневному лимиту.`);
      }
    } catch {
      setProfile(previous);
      setError("Не удалось сохранить уведомления. Попробуйте ещё раз.");
    } finally {
      setSavingNotification("");
    }
  }
  async function disableAutoRenew() {
    setDisablingAutoRenew(true);
    setError("");
    try {
      await apiFetch("/api/billing/autorenew/disable", { method: "POST" });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось отключить автопродление.");
    } finally {
      setDisablingAutoRenew(false);
    }
  }
  async function confirmDelete() {
    setDeleting(true);
    try {
      await apiFetch("/api/profile", { method: "DELETE" });
      router.push("/login?deleted=1");
    } catch {
      setDeleting(false);
      setDeleteConfirm(false);
      setError("Не удалось удалить аккаунт. Попробуйте ещё раз.");
    }
  }
  // 12 attempts * 3s = 36s is too little for a real round trip to the Max app
  // and back (open Max, find the chat, press "Start", return to the browser),
  // especially on a cold app start. Polling used to give up quietly,
  // and the checkmark "hung" in the unlinked state until a manual page refresh.
  // 40 attempts * 3s = 2 minutes, with headroom for the real user flow.
  const linkPollMaxAttempts = 40;
  function startMaxLinkPolling() {
    setMaxLinking(true);
    if (maxPollRef.current) window.clearTimeout(maxPollRef.current);
    let attempts = 0;
    const poll = () => {
      attempts += 1;
      apiFetch<{ maxLinked?: boolean }>("/api/notifications/max/start-link")
        .then((data) => {
          if (data?.maxLinked) { setMaxLinking(false); void load(); return; }
          if (attempts < linkPollMaxAttempts) maxPollRef.current = window.setTimeout(poll, 3000);
          else setMaxLinking(false);
        })
        .catch(() => {
          if (attempts < linkPollMaxAttempts) maxPollRef.current = window.setTimeout(poll, 3000);
          else setMaxLinking(false);
        });
    };
    maxPollRef.current = window.setTimeout(poll, 3000);
  }
  useEffect(() => () => { if (maxPollRef.current) window.clearTimeout(maxPollRef.current); }, []);
  function startTgLinkPolling() {
    setTgLinking(true);
    if (tgPollRef.current) window.clearTimeout(tgPollRef.current);
    let attempts = 0;
    const poll = () => {
      attempts += 1;
      apiFetch<{ telegramLinked?: boolean }>("/api/notifications/telegram/start-link")
        .then((data) => {
          if (data?.telegramLinked) { setTgLinking(false); void load(); return; }
          if (attempts < linkPollMaxAttempts) tgPollRef.current = window.setTimeout(poll, 3000);
          else setTgLinking(false);
        })
        .catch(() => {
          if (attempts < linkPollMaxAttempts) tgPollRef.current = window.setTimeout(poll, 3000);
          else setTgLinking(false);
        });
    };
    tgPollRef.current = window.setTimeout(poll, 3000);
  }
  useEffect(() => () => { if (tgPollRef.current) window.clearTimeout(tgPollRef.current); }, []);
  // Manual check: in case the user came back from Max/Telegram
  // after more than 2 minutes and auto-polling has already given up; we do not make them reload
  // the page by hand.
  async function recheckMaxLink() {
    setMaxLinking(true);
    try {
      await load();
    } finally {
      setMaxLinking(false);
    }
  }
  async function recheckTgLink() {
    setTgLinking(true);
    try {
      await load();
    } finally {
      setTgLinking(false);
    }
  }
  async function unlinkTelegram() {
    if (tgUnlinking) return;
    setTgUnlinking(true);
    setMaxNotice("");
    try {
      await apiFetch("/api/notifications/telegram/unlink", { method: "POST" });
      setMaxNotice("Уведомления Telegram отвязаны. Повторное подключение бонус не начислит.");
      await load();
    } catch (e) {
      const payload = e instanceof ApiError ? (e.payload as { code?: string; hoursLeft?: number } | null) : null;
      if (payload?.code === "cooldown") {
        const hrs = payload.hoursLeft ?? 24;
        setMaxNotice(`Отвязать уведомления Telegram можно только через сутки после подключения (осталось ~${hrs} ч).`);
      } else {
        setMaxNotice(e instanceof Error ? e.message : "Не удалось отвязать Telegram.");
      }
    } finally {
      setTgUnlinking(false);
    }
  }
  async function unlinkMax() {
    if (maxUnlinking) return;
    setMaxUnlinking(true);
    setMaxNotice("");
    try {
      await apiFetch("/api/notifications/max/unlink", { method: "POST" });
      setMaxNotice("Уведомления отвязаны. При повторном подключении бонусные сообщения повторно не начислятся.");
      await load();
    } catch (e) {
      const payload = e instanceof ApiError ? (e.payload as { code?: string; hoursLeft?: number } | null) : null;
      if (payload?.code === "cooldown") {
        const hrs = payload.hoursLeft ?? 24;
        setMaxNotice(`Отвязать уведомления можно только через сутки после подключения (осталось ~${hrs} ч). Повторное подключение бонусные сообщения не начислит.`);
      } else {
        setMaxNotice(e instanceof Error ? e.message : "Не удалось отвязать уведомления.");
      }
    } finally {
      setMaxUnlinking(false);
    }
  }
  useEffect(() => {
    if (!maxNotice) return;
    const t = window.setTimeout(() => setMaxNotice(""), 9000);
    return () => window.clearTimeout(t);
  }, [maxNotice]);
  // The user goes to Max and comes back to the tab: reload the profile
  // to pick up the fresh link without a manual refresh.
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible" && profile && !profile.maxLinked) void load();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [profile?.maxLinked]);
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const saved = window.localStorage.getItem("ms_theme");
    if (saved === "light" || saved === "dark" || saved === "auto") {
      setTheme(saved as "light" | "dark" | "auto");
    }
  }, []);
  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem("ms_theme", theme);
    if (theme === "auto") {
      const prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
      document.documentElement.dataset.theme = prefersDark ? "dark" : "light";
      return;
    }
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  useEffect(() => {
    const check = () => setHeaderMobile(window.innerWidth < 768);
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, []);
  const activeModes = profile?.activeModes ?? [];
  const stats = profile?.stats ?? { dialogs: 0, messages: 0, liveMessages: 0, tokens: 0 };
  const billing = profile?.billing;
  const subscription = billing?.subscription;
  const payments = billing?.payments ?? [];
  const isAdmin = ["owner", "admin", "billing_admin", "content_admin", "support"].includes(profile?.user.role || "");
  const notificationContacts = profile?.notifications?.contacts ?? [];
  const notificationConsents = profile?.notifications?.consents ?? [];
  const notificationInbox = profile?.notifications?.inbox ?? [];
  const hasConsent = (channel: string, consentType: "service" | "marketing") =>
    notificationConsents.some((item) => item.channel === channel && item.consentType === consentType && item.status === "granted");
  const serviceEmailAvailable = notificationContacts.some((item) => item.channel === "email");
  const servicePhoneAvailable = notificationContacts.some((item) => item.channel === "phone" && item.verified);
  return (
    <div className="ms-page">
      <header style={{
        height: headerMobile ? 56 : 64,
        background: "var(--card)",
        borderBottom: "1px solid var(--line)",
        display: "flex",
        alignItems: "center",
        gap: 8,
        padding: `0 ${headerMobile ? 14 : 20}px`,
        flexShrink: 0,
        position: "sticky",
        top: 0,
        zIndex: 20,
      }}>
        <BrandLogo priority />
        <nav style={{
          marginLeft: "auto",
          display: "flex",
          alignItems: "center",
          gap: headerMobile ? 6 : 10,
          flexShrink: 0,
          flexWrap: "nowrap",
        }}>
          <Link href="/chat" style={{
            display: "inline-flex",
            alignItems: "center",
            height: headerMobile ? 34 : 38,
            padding: `0 ${headerMobile ? 12 : 16}px`,
            borderRadius: 10,
            background: "#1D9E75",
            color: "#fff",
            fontSize: 13,
            fontWeight: 600,
            textDecoration: "none",
            whiteSpace: "nowrap",
            flexShrink: 0,
          }}>
            В чат
          </Link>
          {isAdmin && (
            <Link href="/admin" style={{
              display: "inline-flex",
              alignItems: "center",
              height: headerMobile ? 34 : 38,
              padding: `0 ${headerMobile ? 12 : 16}px`,
              borderRadius: 10,
              border: "1px solid var(--line)",
              background: "var(--card)",
              color: "var(--foreground)",
              fontSize: 13,
              fontWeight: 500,
              textDecoration: "none",
              whiteSpace: "nowrap",
              flexShrink: 0,
            }}>
              Админ
            </Link>
          )}
          <button onClick={logout} style={{
            display: "inline-flex",
            alignItems: "center",
            height: headerMobile ? 34 : 38,
            padding: `0 ${headerMobile ? 12 : 16}px`,
            borderRadius: 10,
            border: "1px solid var(--line)",
            background: "var(--card)",
            color: "var(--foreground)",
            fontSize: 13,
            fontWeight: 500,
            cursor: "pointer",
            whiteSpace: "nowrap",
            fontFamily: "inherit",
            flexShrink: 0,
          }}>
            Выйти
          </button>
        </nav>
      </header>
      <main style={{ flex: 1, padding: "32px 16px", maxWidth: 960, margin: "0 auto", width: "100%" }}>
        {error && <div className="ms-error-box" style={{ marginBottom: 16 }}>{error}</div>}
        {!profile && !error && (
          <div className="ms-info-box" style={{ maxWidth: 400 }}>Загрузка профиля...</div>
        )}
        {profile && (
          <div style={{ display: "grid", gridTemplateColumns: "200px minmax(0,1fr)", gap: 24, alignItems: "start" }} className="ms-profile-grid">
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 12, minWidth: 0 }} className="ms-profile-identity">
              <Avatar email={profile.user.email} />
              <div style={{ textAlign: "center" }}>
                <div style={{ fontWeight: 600, fontSize: 15 }}>
                  {profile.user.email || `User ${profile.user.id}`}
                </div>
                <div style={{ marginTop: 6, display: "flex", gap: 6, justifyContent: "center", flexWrap: "wrap" }}>
                  <span className="ms-chip" style={{ fontSize: 12 }}>{profile.user.role}</span>
                  <span
                    className="ms-chip"
                    style={{
                      fontSize: 12,
                      background: profile.user.status === "active" ? "var(--soft)" : "var(--danger-bg)",
                      color: profile.user.status === "active" ? "var(--accent-strong)" : "#8a3333",
                      borderColor: profile.user.status === "active" ? "#cfe5e1" : "var(--danger-line)",
                    }}
                  >
                    {profile.user.status === "active" ? "активен" : "заблокирован"}
                  </span>
                </div>
              </div>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 14, minWidth: 0 }}>
              <div className="card" style={{ padding: 20 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
                  <div>
                    <div style={{ fontSize: 13, color: "var(--muted)", fontWeight: 700, marginBottom: 4 }}>Статус доступа</div>
                    <div style={{ fontWeight: 600, fontSize: 18 }}>
                      {profile.hasAccess ? "Доступ активен" : "Нет активного доступа"}
                    </div>
                    {profile.activeTo && (
                      <div style={{ fontSize: 13, color: "var(--muted)", marginTop: 4 }}>
                        до {new Date(profile.activeTo).toLocaleString("ru-RU")}
                      </div>
                    )}
                  </div>
                  {!isAdmin && (
                    <Link
                      href="/access?force=promo"
                      style={{
                        display: "inline-flex",
                        alignItems: "center",
                        justifyContent: "center",
                        height: headerMobile ? 34 : 38,
                        padding: `0 ${headerMobile ? 12 : 16}px`,
                        borderRadius: 10,
                        background: "#1D9E75",
                        color: "#fff",
                        fontSize: 13,
                        fontWeight: 600,
                        textDecoration: "none",
                        whiteSpace: "nowrap",
                        flexShrink: 0,
                      }}
                    >
                      Ввести промокод
                    </Link>
                  )}
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10, marginTop: 16 }} className="ms-profile-stats-grid">
                  {[
                    { label: "Активных режимов", value: activeModes.length },
                    { label: "Сообщений", value: stats.messages },
                    { label: "Live-запросов", value: stats.liveMessages },
                  ].map((s) => (
                    <div
                      key={s.label}
                      style={{
                        padding: "12px 14px",
                        background: "var(--soft)",
                        border: "1px solid var(--line)",
                        borderRadius: 14,
                      }}
                    >
                      <div style={{ fontSize: 11, color: "var(--muted)", fontWeight: 700 }}>{s.label}</div>
                      <div style={{ fontSize: 20, fontWeight: 700, marginTop: 4, fontVariantNumeric: "tabular-nums" }}>
                        {s.value}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              <div className="card" style={{ padding: 20 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                  <div style={{ fontWeight: 600, fontSize: 14 }}>Активные режимы</div>
                  <Link href="/chat" style={{ fontSize: 13, color: "var(--accent-strong)", fontWeight: 700 }}>
                    Открыть чат →
                  </Link>
                </div>
                {activeModes.length === 0 ? (
                  <p className="muted" style={{ margin: 0 }}>Нет активных режимов</p>
                ) : (
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                    {activeModes.map((mode, i) => (
                      <div
                        key={`${mode.modeId}-${i}`}
                        className="ms-chip"
                        style={{ position: "relative" }}
                        title={mode.activeTo ? `до ${new Date(mode.activeTo).toLocaleDateString("ru-RU")}${daysLeft(mode.activeTo)}` : "без срока"}
                      >
                        {mode.modeName}
                        {mode.activeTo && (
                          <span style={{ marginLeft: 6, fontSize: 11, opacity: 0.7 }}>
                            {daysLeft(mode.activeTo).replace(" · ", "")}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
	                )}
	              </div>
	              {(subscription?.id || payments.length > 0) && (
	                <div className="card" style={{ padding: 20 }}>
	                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
	                    <div>
	                      <div style={{ fontWeight: 600, fontSize: 14 }}>Подписка и платежи</div>
	                      {subscription?.id ? (
	                        <p className="muted" style={{ margin: "6px 0 0", fontSize: 13, lineHeight: 1.45 }}>
	                          {subscription.tariffName || "Тариф"} · {subscription.status || "status unknown"}
	                          {subscription.activeTo ? ` · до ${new Date(subscription.activeTo).toLocaleDateString("ru-RU")}` : ""}
	                        </p>
	                      ) : (
	                        <p className="muted" style={{ margin: "6px 0 0", fontSize: 13 }}>Активной подписки нет.</p>
	                      )}
	                    </div>
	                    {subscription?.autoRenewEnabled && (
	                      <button
	                        type="button"
	                        disabled={disablingAutoRenew}
	                        onClick={() => void disableAutoRenew()}
	                        style={{ height: 36, padding: "0 14px", borderRadius: 10, border: "1px solid var(--line)", background: "var(--card)", color: "var(--foreground)", fontSize: 13, fontWeight: 600, cursor: disablingAutoRenew ? "wait" : "pointer", fontFamily: "inherit" }}
	                      >
	                        {disablingAutoRenew ? "Отключаем…" : "Отключить автопродление"}
	                      </button>
	                    )}
	                  </div>
	                  {subscription?.id && (
	                    <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10, marginTop: 14 }} className="ms-profile-stats-grid">
	                      {[
	                        { label: "Автопродление", value: subscription.autoRenewEnabled ? "включено" : "выключено" },
	                        { label: "Метод", value: subscription.paymentMethodStatus ? `${subscription.paymentMethodType || "method"} · ${subscription.paymentMethodStatus}` : "не сохранён" },
	                        { label: "Попытки", value: String(subscription.consecutiveFailures ?? 0) },
	                      ].map((s) => (
	                        <div key={s.label} style={{ padding: "12px 14px", background: "var(--soft)", border: "1px solid var(--line)", borderRadius: 14 }}>
	                          <div style={{ fontSize: 11, color: "var(--muted)", fontWeight: 700 }}>{s.label}</div>
	                          <div style={{ fontSize: 13, fontWeight: 700, marginTop: 4 }}>{s.value}</div>
	                        </div>
	                      ))}
	                    </div>
	                  )}
	                  {payments.length > 0 && (
	                    <div style={{ marginTop: 16, display: "grid", gap: 8 }}>
	                      {payments.slice(0, 5).map((payment) => (
	                        <div key={payment.id} style={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 10, padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 10, background: "color-mix(in oklab, var(--card) 88%, transparent)" }}>
	                          <div style={{ minWidth: 0 }}>
	                            <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{payment.tariffName || payment.paymentId}</div>
	                            <div style={{ marginTop: 3, color: "var(--muted)", fontSize: 12 }}>
	                              {new Date(payment.createdAt).toLocaleDateString("ru-RU")} · {payment.status}
	                              {payment.autoRenewRequested ? " · автопродление" : " · разово"}
	                            </div>
	                          </div>
	                          <div style={{ fontSize: 13, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>{payment.amount} {payment.currency}</div>
	                        </div>
	                      ))}
	                    </div>
	                  )}
	                </div>
	              )}
              <div className="card" style={{ padding: 20 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16 }}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontWeight: 600, fontSize: 14 }}>Убирать личные данные из старой переписки</div>
                    <p className="muted" style={{ margin: "6px 0 0", fontSize: 13, lineHeight: 1.45 }}>
                      Хотите анонимно общаться - оставьте включенным. Хотите точнее - выключите.
                    </p>
                  </div>
                  <button
                    type="button"
                    role="switch"
                    aria-label="Убирать личные данные из старой переписки"
                    aria-checked={profile.allowMessageAnonymization}
                    disabled={savingAnonymization}
                    onClick={() => void updateMessageAnonymization(!profile.allowMessageAnonymization)}
                    style={{
                      width: 54,
                      height: 30,
                      padding: 3,
                      border: `1px solid ${profile.allowMessageAnonymization ? "var(--accent)" : "var(--line)"}`,
                      borderRadius: 999,
                      background: profile.allowMessageAnonymization ? "#1D9E75" : "var(--soft)",
                      cursor: savingAnonymization ? "wait" : "pointer",
                      opacity: savingAnonymization ? 0.65 : 1,
                      transition: "all 0.15s ease",
                      flexShrink: 0,
                    }}
                  >
                    <span
                      style={{
                        display: "block",
                        width: 22,
                        height: 22,
                        borderRadius: "50%",
                        background: "#fff",
                        transform: profile.allowMessageAnonymization ? "translateX(24px)" : "translateX(0)",
                        transition: "transform 0.15s ease",
                        boxShadow: "0 2px 6px rgba(0,0,0,0.18)",
                      }}
                    />
                  </button>
                </div>
                <div style={{ marginTop: 10, fontSize: 12, color: "var(--muted)" }}>
                  {profile.allowMessageAnonymization
                    ? "Включено: имена в старых сообщениях будут заменены."
                    : "Выключено: старые сообщения сохранят имена, и обращения будут точнее."}
                </div>
              </div>
              <div className="card" style={{ padding: 20 }}>
                <div style={{ fontWeight: 600, fontSize: 14 }}>Что присылать</div>
                <div style={{ marginTop: 10 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", gap: 14, alignItems: "center", padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 12 }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: 13, fontWeight: 700 }}>Уведомления в Max</div>
                      <div className="muted" style={{ marginTop: 3, fontSize: 12, lineHeight: 1.4 }}>
                        {profile.maxLinked ? "Подключено. Акции, продление доступа, новые способы применения." : "Акции, продление доступа, новые способы применения."}
                      </div>
                    </div>
                    {profile.maxLinked ? (
                      <button
                        type="button"
                        onClick={unlinkMax}
                        disabled={maxUnlinking}
                        aria-label="Отвязать уведомления в Max"
                        title="Отвязать уведомления в Max"
                        style={{ width: 54, height: 30, padding: 3, border: "1px solid var(--accent)", borderRadius: 999, background: "#1D9E75", flexShrink: 0, cursor: maxUnlinking ? "default" : "pointer", opacity: maxUnlinking ? 0.6 : 1 }}
                      >
                        <span style={{ display: "block", width: 22, height: 22, borderRadius: "50%", background: "#fff", transform: "translateX(24px)", transition: "transform 0.15s ease" }} />
                      </button>
                    ) : (
                      <a
                        href={profile.maxBotLink ?? "#"}
                        target="_blank"
                        rel="noopener noreferrer"
                        aria-label="Подключить уведомления в Max"
                        onClick={startMaxLinkPolling}
                        style={{ textDecoration: "none", flexShrink: 0 }}
                      >
                        <div style={{ width: 54, height: 30, padding: 3, border: "1px solid var(--line)", borderRadius: 999, background: "var(--soft)", cursor: "pointer" }}>
                          <span style={{ display: "block", width: 22, height: 22, borderRadius: "50%", background: "#fff", transform: "translateX(0)", transition: "transform 0.15s ease" }} />
                        </div>
                      </a>
                    )}
                  </div>
                  {!profile.maxLinked && (
                    <div className="muted" style={{ marginTop: 8, fontSize: 12, lineHeight: 1.45 }}>
                      {maxLinking
                        ? "Ожидаем подтверждения из Max… нажмите «Старт» в чате бота — статус обновится здесь автоматически."
                        : "При подключении начислим бонусные сообщения к дневному лимиту, а важные события будут приходить в мессенджер Max."}
                    </div>
                  )}
                  {profile.maxLinked && !maxNotice && (
                    <div className="muted" style={{ marginTop: 8, fontSize: 12, lineHeight: 1.45 }}>
                      Чтобы отключить — нажмите на переключатель. Отвязать можно через сутки после подключения; бонусные сообщения повторно не начисляются.
                      {(profile.maxAlsoLinkedTo?.length ?? 0) > 0 && (
                        <> Этот Max также привязан к: {profile.maxAlsoLinkedTo!.join(", ")}.</>
                      )}
                    </div>
                  )}
                  {maxNotice && (
                    <div role="status" aria-live="polite" style={{ marginTop: 8, padding: "9px 12px", borderRadius: 10, background: "var(--soft)", border: "1px solid var(--accent)", fontSize: 12, lineHeight: 1.45, color: "var(--accent-strong)" }}>
                      {maxNotice}
                    </div>
                  )}
                  {/* Telegram: linking works like Max */}
                  <div style={{ marginTop: 10, display: "flex", justifyContent: "space-between", gap: 14, alignItems: "center", padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 12 }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: 13, fontWeight: 700 }}>Уведомления в Telegram</div>
                      <div className="muted" style={{ marginTop: 3, fontSize: 12, lineHeight: 1.4 }}>
                        {profile.telegramLinked
                          ? "Подключено. Акции, продление доступа, новые способы применения."
                          : tgLinking
                            ? "Ожидаем подтверждения… нажмите «Старт» в чате бота — статус обновится автоматически."
                            : "Акции, продление доступа, новые способы применения."}
                      </div>
                    </div>
                    {profile.telegramLinked ? (
                      <button
                        type="button"
                        onClick={unlinkTelegram}
                        disabled={tgUnlinking}
                        aria-label="Отвязать уведомления в Telegram"
                        title="Отвязать уведомления в Telegram"
                        style={{ width: 54, height: 30, padding: 3, border: "1px solid var(--accent)", borderRadius: 999, background: "#1D9E75", flexShrink: 0, cursor: tgUnlinking ? "default" : "pointer", opacity: tgUnlinking ? 0.6 : 1 }}
                      >
                        <span style={{ display: "block", width: 22, height: 22, borderRadius: "50%", background: "#fff", transform: "translateX(24px)", transition: "transform 0.15s ease" }} />
                      </button>
                    ) : (
                      <a
                        href={profile.telegramBotLink ?? "#"}
                        target="_blank"
                        rel="noopener noreferrer"
                        aria-label="Подключить уведомления в Telegram"
                        onClick={startTgLinkPolling}
                        style={{ textDecoration: "none", flexShrink: 0 }}
                      >
                        <div style={{ width: 54, height: 30, padding: 3, border: "1px solid var(--line)", borderRadius: 999, background: "var(--soft)", cursor: "pointer" }}>
                          <span style={{ display: "block", width: 22, height: 22, borderRadius: "50%", background: "#fff", transform: "translateX(0)", transition: "transform 0.15s ease" }} />
                        </div>
                      </a>
                    )}
                  </div>
                  <div className="muted" style={{ marginTop: 8, fontSize: 11, lineHeight: 1.4 }}>
                    За первое подключение каждого канала начисляются бонусные сообщения к дневному лимиту. Повторное подключение бонус не даёт.
                  </div>
                </div>
                {notificationInbox.length > 0 && (
                  <div style={{ marginTop: 16, display: "grid", gap: 8 }}>
                    <div style={{ fontSize: 12, color: "var(--muted)", fontWeight: 700 }}>Последние пуши</div>
                    {notificationInbox.slice(0, 1).map((item) => (
                      <div key={item.id} style={{ padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 10, background: item.status === "unread" ? "var(--soft)" : "color-mix(in oklab, var(--card) 88%, transparent)" }}>
                        <div style={{ display: "flex", justifyContent: "space-between", gap: 10, alignItems: "baseline" }}>
                          <div style={{ fontSize: 13, fontWeight: 700 }}>{item.title}</div>
                          <div style={{ color: "var(--muted)", fontSize: 11, whiteSpace: "nowrap" }}>{new Date(item.createdAt).toLocaleDateString("ru-RU")}</div>
                        </div>
                        <div className="muted" style={{ marginTop: 4, fontSize: 12, lineHeight: 1.4 }}>
                          <MessageContent content={item.body} />
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div className="card" style={{ padding: 20 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                  <div style={{ fontWeight: 600, fontSize: 14 }}>Тема оформления</div>
                </div>
                <div style={{ display: "flex", gap: 8 }} className="ms-theme-picker">
                  {(["light", "dark", "auto"] as const).map((t) => (
                    <button
                      key={t}
                      type="button"
                      onClick={() => setTheme(t)}
                      style={{
                        flex: 1,
                        minHeight: 44,
                        border: `1px solid ${theme === t ? "var(--accent)" : "var(--line)"}`,
                        borderRadius: 13,
                        background: theme === t ? "var(--soft)" : "color-mix(in oklab, var(--card) 88%, transparent)",
                        color: theme === t ? "var(--accent-strong)" : "var(--muted)",
                        fontWeight: 600,
                        fontSize: 13,
                        cursor: "pointer",
                        transition: "all 0.15s ease",
                      }}
                    >
                      {t === "light" ? "☀️ Светлая" : t === "dark" ? "🌙 Тёмная" : "⚙️ Авто"}
                    </button>
                  ))}
                </div>
              </div>
              <div className="card" style={{ padding: 0, overflow: "hidden" }}>
                {[
                  { label: "Email", value: profile.user.email || `User #${profile.user.id}` },
                  { label: "ID пользователя", value: `#${profile.user.id}` },
                  { label: "Диалогов", value: String(stats.dialogs) },
                  { label: "Токенов использовано", value: stats.tokens.toLocaleString("ru-RU") },
                ].map((row, i, arr) => (
                  <div
                    key={row.label}
                    style={{
                      padding: "14px 18px",
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      borderBottom: i < arr.length - 1 ? "1px solid var(--line)" : "none",
                      gap: 12,
                    }}
                  >
                    <div style={{ fontSize: 13, color: "var(--muted)", fontWeight: 700 }}>{row.label}</div>
                    <div style={{ fontSize: 13, fontWeight: 500, maxWidth: "60%", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {row.value}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
        {profile && (
          <div style={{ maxWidth: 960, margin: "24px auto 0", padding: "0 16px", textAlign: "right" }}>
            <button
              onClick={() => setDeleteConfirm(true)}
              style={{
                fontSize: 13,
                color: "var(--danger, #cc3333)",
                background: "none",
                border: "none",
                cursor: "pointer",
                opacity: 0.7,
                fontFamily: "inherit",
                padding: "4px 0",
              }}
            >
              Удалить аккаунт
            </button>
          </div>
        )}
        {deleteConfirm && (
          <div style={{
            position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)",
            display: "flex", alignItems: "center", justifyContent: "center", zIndex: 100,
          }}>
            <div className="card" style={{ maxWidth: 420, width: "90%", padding: 28 }}>
              <div style={{ fontWeight: 700, fontSize: 17, marginBottom: 12 }}>Удалить аккаунт?</div>
              <p style={{ fontSize: 14, color: "var(--muted)", marginBottom: 20, lineHeight: 1.5 }}>
                Это действие необратимо. Все ваши диалоги и доступы будут удалены.
                Мы уважаем ваш выбор — если удалили, значит так надо.
              </p>
              <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
                <button
                  onClick={() => setDeleteConfirm(false)}
                  disabled={deleting}
                  style={{
                    padding: "10px 18px", borderRadius: 10, border: "1px solid var(--line)",
                    background: "var(--card)", color: "var(--foreground)", fontSize: 14,
                    cursor: "pointer", fontFamily: "inherit",
                  }}
                >
                  Отмена
                </button>
                <button
                  onClick={confirmDelete}
                  disabled={deleting}
                  style={{
                    padding: "10px 18px", borderRadius: 10, border: "none",
                    background: "var(--danger, #cc3333)", color: "#fff", fontSize: 14,
                    fontWeight: 600, cursor: deleting ? "not-allowed" : "pointer",
                    opacity: deleting ? 0.7 : 1, fontFamily: "inherit",
                  }}
                >
                  {deleting ? "Удаляем..." : "Удалить навсегда"}
                </button>
              </div>
            </div>
          </div>
        )}
      </main>
      <style>{`
        @media (max-width: 640px) {
          .ms-profile-grid {
            grid-template-columns: 1fr !important;
          }
          .ms-profile-identity {
            flex-direction: row !important;
            align-items: center !important;
            gap: 16px !important;
          }
          .ms-profile-stats-grid {
            grid-template-columns: 1fr 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
