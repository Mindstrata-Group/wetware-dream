"use client";

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";

type AdminPaymentRow = {
  id: number;
  userId: number;
  email: string;
  subscriptionId?: number;
  tariffName: string;
  paymentId: string;
  status: string;
  amount: string;
  currency: string;
  isRecurring: boolean;
  autoRenewRequested: boolean;
  renewalAttempt: number;
  createdAt: string;
  paidAt?: string;
  cancellationReason?: string;
  autoRenewEnabled?: boolean;
  subscriptionStatus?: string;
  nextRetryAt?: string;
  graceUntil?: string;
  consecutiveFailures?: number;
  paymentMethodStatus?: string;
  paymentMethodType?: string;
};

type BillingRunRow = {
  id: number;
  runKey: string;
  runType: string;
  source: string;
  dryRun: boolean;
  status: string;
  totals: Record<string, unknown>;
  error?: string;
  startedAt: string;
  finishedAt?: string;
};

type YooKassaConfig = {
  testModeEnabled: boolean;
  activeMode: "live" | "test";
  activeConfigured: boolean;
  liveConfigured: boolean;
  liveShopIdMasked?: string;
  testConfigured: boolean;
  testShopId?: string;
  testSecretKeyMasked?: string;
};

type AccessRecoveryItem = {
  invoiceId: number;
  userId: number;
  email: string;
  tariffName: string;
  paymentId: string;
  amount: string;
  currency: string;
  status: string;
  subscriptionMonths: number;
  accessCount: number;
  usageCount: number;
  accessActiveTo?: string;
  transferableWithoutRisk: boolean;
};

// AdminPaymentsTab is the single entry point for admin payments via YooKassa.
// For the MVP: arbitrary amount, description, return URL, card binding flag.
// The webhook is handled by the server (`/webhooks/yookassa/<secret-path>`).
//
// Why it is in the admin panel: at launch payments are needed only by admins to test
// charges/refunds and for manual charges. Once the scenario is confirmed,
// the same handler will move into the user flow (tariff subscriptions).
export function AdminPaymentsTab() {
  const [amountRub, setAmountRub] = useState("100.00");
  const [testChargeRub, setTestChargeRub] = useState("100.00");
  const [savePaymentMethod, setSavePaymentMethod] = useState(true);
  const [returnUrl, setReturnUrl] = useState("https://mindstrata.ru/admin");
  const [description, setDescription] = useState("Админ-платеж Mindstrata");
  const [loading, setLoading] = useState(false);
  const [listLoading, setListLoading] = useState(false);
  const [runningRenewals, setRunningRenewals] = useState(false);
  const [rowActionId, setRowActionId] = useState<number | null>(null);
  const [confirmationUrl, setConfirmationUrl] = useState("");
  const [payments, setPayments] = useState<AdminPaymentRow[]>([]);
  const [billingRuns, setBillingRuns] = useState<BillingRunRow[]>([]);
  const [config, setConfig] = useState<YooKassaConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(false);
  const [configSaving, setConfigSaving] = useState(false);
  const [testModeEnabled, setTestModeEnabled] = useState(false);
  const [testShopId, setTestShopId] = useState("");
  const [testSecretKey, setTestSecretKey] = useState("");
  const [recoveryQuery, setRecoveryQuery] = useState("");
  const [recoveryTarget, setRecoveryTarget] = useState("");
  const [recoveryItems, setRecoveryItems] = useState<AccessRecoveryItem[]>([]);
  const [recoveryLoading, setRecoveryLoading] = useState(false);
  const [recoveryActionId, setRecoveryActionId] = useState<number | null>(null);
  const [error, setError] = useState("");

  async function loadPayments() {
    setListLoading(true);
    setError("");
    try {
      const res = await apiFetch<{ payments?: AdminPaymentRow[]; billingRuns?: BillingRunRow[] }>("/api/admin/payments");
      setPayments(res.payments || []);
      setBillingRuns(res.billingRuns || []);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка загрузки платежей");
    } finally {
      setListLoading(false);
    }
  }

  async function loadConfig() {
    setConfigLoading(true);
    setError("");
    try {
      const res = await apiFetch<YooKassaConfig>("/api/admin/payments/yookassa/config");
      setConfig(res);
      setTestModeEnabled(Boolean(res.testModeEnabled));
      setTestShopId(res.testShopId || "");
      setTestSecretKey("");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка загрузки настроек YooKassa");
    } finally {
      setConfigLoading(false);
    }
  }

  useEffect(() => {
    void loadPayments();
    void loadConfig();
  }, []);

  async function saveConfig(clearSecret = false) {
    setConfigSaving(true);
    setError("");
    try {
      const payload: Record<string, unknown> = {
        testModeEnabled,
        testShopId,
        clearTestSecret: clearSecret,
      };
      if (testSecretKey.trim()) {
        payload.testSecretKey = testSecretKey.trim();
      }
      const res = await apiFetch<YooKassaConfig>("/api/admin/payments/yookassa/config", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      setConfig(res);
      setTestModeEnabled(Boolean(res.testModeEnabled));
      setTestShopId(res.testShopId || "");
      setTestSecretKey("");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка сохранения настроек YooKassa");
    } finally {
      setConfigSaving(false);
    }
  }

  async function createPayment() {
    setLoading(true);
    setError("");
    setConfirmationUrl("");
    try {
      const res = await apiFetch<{ confirmationUrl?: string }>(
        "/api/admin/payments/yookassa/create",
        {
          method: "POST",
          body: JSON.stringify({
            amountRub: Number(amountRub),
            savePaymentMethod,
            returnUrl,
            description,
          }),
        }
      );
      setConfirmationUrl(res.confirmationUrl || "");
      await loadPayments();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Ошибка создания платежа";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }

  async function runDueRenewals() {
    setRunningRenewals(true);
    setError("");
    try {
      await apiFetch("/api/admin/payments/yookassa/run-due-renewals", { method: "POST" });
      await loadPayments();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка запуска автопродлений");
    } finally {
      setRunningRenewals(false);
    }
  }

  async function revokeSubscription(payment: AdminPaymentRow) {
    if (!payment.subscriptionId && !payment.userId) {
      setError("В строке нет пользователя или подписки для снятия доступа");
      return;
    }
    if (!window.confirm(`Снять активную подписку и доступ у ${payment.email || `user #${payment.userId}`}?`)) {
      return;
    }
    setRowActionId(payment.id);
    setError("");
    try {
      await apiFetch("/api/admin/payments/subscriptions/revoke", {
        method: "POST",
        body: JSON.stringify({
          userId: payment.userId,
          subscriptionId: payment.subscriptionId || 0,
          reason: "admin_revoked",
        }),
      });
      await loadPayments();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка снятия доступа");
    } finally {
      setRowActionId(null);
    }
  }

  async function testAutoCharge(payment: AdminPaymentRow) {
    if (!payment.subscriptionId && !payment.userId) {
      setError("В строке нет пользователя или подписки для тестового списания");
      return;
    }
    setRowActionId(payment.id);
    setError("");
    try {
      await apiFetch("/api/admin/payments/yookassa/test-charge", {
        method: "POST",
        body: JSON.stringify({
          userId: payment.userId,
          subscriptionId: payment.subscriptionId || 0,
          amountRub: Number(testChargeRub),
          description: "Тестовое автосписание Mindstrata",
        }),
      });
      await loadPayments();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка тестового автосписания");
    } finally {
      setRowActionId(null);
    }
  }

  async function searchAccessRecovery() {
    const q = recoveryQuery.trim();
    if (!q) {
      setRecoveryItems([]);
      return;
    }
    setRecoveryLoading(true);
    setError("");
    try {
      const res = await apiFetch<{ items?: AccessRecoveryItem[] }>(`/api/admin/payments/access-recovery?q=${encodeURIComponent(q)}`);
      setRecoveryItems(res.items || []);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка поиска доступа");
    } finally {
      setRecoveryLoading(false);
    }
  }

  async function transferRecoveredAccess(item: AccessRecoveryItem) {
    const target = recoveryTarget.trim();
    if (!target) {
      setError("Укажите userId или email, куда передать доступ");
      return;
    }
    setRecoveryActionId(item.invoiceId);
    setError("");
    try {
      const targetUserId = /^\d+$/.test(target) ? Number(target) : 0;
      const targetEmail = targetUserId ? "" : target;
      await apiFetch("/api/admin/payments/access-recovery", {
        method: "POST",
        body: JSON.stringify({
          invoiceId: item.invoiceId,
          targetUserId,
          targetEmail,
          reason: "lost_cookie_recovery",
        }),
      });
      await searchAccessRecovery();
      await loadPayments();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Ошибка передачи доступа");
    } finally {
      setRecoveryActionId(null);
    }
  }

  return (
    <div className="card ms-admin-card">
      <div className="ms-admin-card-head">
        <h2>Платежи ЮKassa</h2>
      </div>
      <div className="ms-admin-stack" style={{ gap: 10 }}>
        <div style={{ display: "grid", gap: 10, padding: 12, border: "1px solid var(--line)", borderRadius: 10, background: "var(--soft)" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10, flexWrap: "wrap" }}>
            <div>
              <div style={{ fontWeight: 700, fontSize: 14 }}>Режим YooKassa</div>
              <div style={{ color: "var(--muted)", fontSize: 12, marginTop: 4 }}>
                {configLoading
                  ? "Загружаем настройки..."
                  : `Активен: ${config?.activeMode === "test" ? "тестовый магазин" : "боевой магазин"} · ${config?.activeConfigured ? "реквизиты есть" : "реквизиты не заданы"}`}
              </div>
            </div>
            <button
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => void loadConfig()}
              disabled={configLoading || configSaving}
            >
              Обновить настройки
            </button>
          </div>
          <label style={{ display: "flex", gap: 8, alignItems: "center" }}>
            <input
              type="checkbox"
              checked={testModeEnabled}
              onChange={(e) => setTestModeEnabled(e.target.checked)}
            />
            Использовать тестовый магазин для новых платежей
          </label>
          <label>
            Test shopId
            <input
              className="ms-input"
              value={testShopId}
              onChange={(e) => setTestShopId(e.target.value)}
              placeholder="test shopId из кабинета YooKassa"
            />
          </label>
          <label>
            Test secret key
            <input
              className="ms-input"
              type="password"
              value={testSecretKey}
              onChange={(e) => setTestSecretKey(e.target.value)}
              placeholder={config?.testSecretKeyMasked || "введите новый ключ"}
            />
          </label>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <button
              className="ms-button ms-button-primary ms-button-xs"
              onClick={() => void saveConfig(false)}
              disabled={configSaving}
            >
              {configSaving ? "Сохраняем..." : "Сохранить режим"}
            </button>
            <button
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => void saveConfig(true)}
              disabled={configSaving}
            >
              Очистить тестовый ключ
            </button>
            <span style={{ color: "var(--muted)", fontSize: 12 }}>
              Live: {config?.liveConfigured ? config.liveShopIdMasked || "настроен" : "не настроен"} · Test: {config?.testConfigured ? config.testSecretKeyMasked || "настроен" : "не настроен"}
            </span>
          </div>
        </div>
        <label>
          Сумма (₽)
          <input
            className="ms-input"
            value={amountRub}
            onChange={(e) => setAmountRub(e.target.value)}
            inputMode="decimal"
          />
        </label>
        <label>
          Описание
          <input
            className="ms-input"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label>
          Return URL (куда вернётся пользователь после оплаты)
          <input
            className="ms-input"
            value={returnUrl}
            onChange={(e) => setReturnUrl(e.target.value)}
          />
        </label>
        <label style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <input
            type="checkbox"
            checked={savePaymentMethod}
            onChange={(e) => setSavePaymentMethod(e.target.checked)}
          />
          Привязать карту для последующих авточарджей (save_payment_method)
        </label>
        <button
          className="ms-button ms-button-primary ms-button-xs"
          onClick={() => void createPayment()}
          disabled={loading}
          style={{ alignSelf: "flex-start", whiteSpace: "nowrap" }}
        >
          {loading ? "Создаём…" : "Создать платёж"}
        </button>
        <button
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={() => void runDueRenewals()}
          disabled={runningRenewals}
          style={{ alignSelf: "flex-start", whiteSpace: "nowrap" }}
        >
          {runningRenewals ? "Проверяем…" : "Запустить автопродления"}
        </button>
        <label style={{ maxWidth: 240 }}>
          Тест автосписания (₽, максимум 500)
          <input
            className="ms-input"
            value={testChargeRub}
            onChange={(e) => setTestChargeRub(e.target.value)}
            inputMode="decimal"
          />
        </label>
        {error && (
          <div style={{ color: "#b00020", fontSize: 13 }}>{error}</div>
        )}
        {confirmationUrl && (
          <a
            href={confirmationUrl}
            target="_blank"
            rel="noreferrer"
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              height: 38,
              padding: "0 16px",
              borderRadius: 10,
              border: "1px solid var(--line)",
              background: "var(--card)",
              color: "var(--foreground)",
              fontSize: 13,
              fontWeight: 500,
              textDecoration: "none",
              whiteSpace: "nowrap",
              fontFamily: "inherit",
              alignSelf: "flex-start",
            }}
          >
            Перейти к оплате
          </a>
        )}
        <div style={{ fontSize: 12, color: "var(--muted)" }}>
          Webhook путь задаётся переменной окружения{" "}
          <code>YOOKASSA_WEBHOOK_PATH</code> на сервере.
        </div>
        <div style={{ display: "grid", gap: 10, padding: 12, border: "1px solid var(--line)", borderRadius: 10, background: "var(--soft)" }}>
          <div>
            <div style={{ fontWeight: 700, fontSize: 14 }}>Восстановить доступ после потери куки</div>
            <div style={{ color: "var(--muted)", fontSize: 12, marginTop: 4 }}>
              Найдите оплату по YooKassa paymentId, invoiceId, userId, email или телефону. Передача доступна только если оплаченный доступ не использовали.
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) minmax(160px, 0.55fr) auto", gap: 8 }} className="ms-profile-grid">
            <input
              className="ms-input"
              value={recoveryQuery}
              onChange={(e) => setRecoveryQuery(e.target.value)}
              placeholder="paymentId, email, телефон, userId"
            />
            <input
              className="ms-input"
              value={recoveryTarget}
              onChange={(e) => setRecoveryTarget(e.target.value)}
              placeholder="куда передать: userId или email"
            />
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => void searchAccessRecovery()}
              disabled={recoveryLoading}
            >
              {recoveryLoading ? "Ищем…" : "Найти"}
            </button>
          </div>
          {recoveryItems.length > 0 && (
            <div style={{ display: "grid", gap: 8 }}>
              {recoveryItems.map((item) => (
                <div key={item.invoiceId} style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: 10, alignItems: "center", padding: "9px 10px", border: "1px solid var(--line)", borderRadius: 8, background: "var(--card)" }}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: 13, fontWeight: 700, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      #{item.invoiceId} · {item.tariffName || item.paymentId} · {item.email || `user #${item.userId}`}
                    </div>
                    <div style={{ color: "var(--muted)", fontSize: 12, marginTop: 3 }}>
                      {item.status} · {item.amount} {item.currency} · доступов {item.accessCount} · использований {item.usageCount}
                    </div>
                  </div>
                  <button
                    type="button"
                    className="ms-button ms-button-primary ms-button-xs"
                    disabled={!item.transferableWithoutRisk || recoveryActionId === item.invoiceId}
                    onClick={() => void transferRecoveredAccess(item)}
                  >
                    {recoveryActionId === item.invoiceId ? "Передаём…" : item.transferableWithoutRisk ? "Передать доступ" : "Уже использовали"}
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
        {billingRuns.length > 0 && (
          <div style={{ marginTop: 12, display: "grid", gap: 8 }}>
            <div style={{ fontWeight: 700, fontSize: 14 }}>Прогоны автопродлений</div>
            {billingRuns.map((run) => {
              const renewals = run.totals?.renewals as Record<string, unknown> | undefined;
              const reconcile = run.totals?.reconcile as Record<string, unknown> | undefined;
              return (
                <div key={run.id} style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: 10, padding: "9px 12px", border: `1px solid ${run.status === "succeeded" ? "var(--line)" : "#b00020"}`, borderRadius: 10, background: "var(--soft)" }}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: 13, fontWeight: 700 }}>
                      {run.runKey}
                      {run.dryRun && <span style={{ marginLeft: 6, color: "var(--muted)", fontWeight: 400 }}>dry-run</span>}
                    </div>
                    <div style={{ color: "var(--muted)", fontSize: 12, marginTop: 3 }}>
                      {run.source} · reconcile: {JSON.stringify(reconcile ?? {})} · renewals: {String(renewals?.processed ?? "—")}
                      {run.error && <span style={{ color: "#b00020" }}> · {run.error}</span>}
                    </div>
                  </div>
                  <div style={{ textAlign: "right", fontSize: 12, color: run.status === "succeeded" ? "var(--muted)" : "#b00020", whiteSpace: "nowrap" }}>
                    {run.status}
                    <div>{run.startedAt ? new Date(run.startedAt).toLocaleString("ru") : ""}</div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
        <div style={{ marginTop: 12, display: "grid", gap: 8 }}>
          <div style={{ fontWeight: 700, fontSize: 14 }}>Последние платежи</div>
          {listLoading && <div style={{ color: "var(--muted)", fontSize: 13 }}>Загружаем платежи…</div>}
          {!listLoading && payments.length === 0 && <div style={{ color: "var(--muted)", fontSize: 13 }}>Платежей пока нет.</div>}
          {payments.map((payment) => (
            <div key={payment.id} style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) auto", gap: 10, padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 10, background: "var(--soft)" }}>
              <div style={{ minWidth: 0 }}>
                <div style={{ fontSize: 13, fontWeight: 700, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {payment.tariffName || payment.paymentId} · {payment.email || `user #${payment.userId}`}
                </div>
                <div style={{ marginTop: 4, color: "var(--muted)", fontSize: 12, lineHeight: 1.45 }}>
                  {payment.status}
                  {payment.autoRenewRequested ? " · autorenew requested" : " · one-time"}
                  {payment.isRecurring ? ` · renewal #${payment.renewalAttempt || 1}` : ""}
                  {payment.autoRenewEnabled ? " · enabled" : ""}
                  {payment.paymentMethodStatus ? ` · ${payment.paymentMethodType}/${payment.paymentMethodStatus}` : ""}
                </div>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 8 }}>
                  <button
                    type="button"
                    className="ms-button ms-button-ghost ms-button-xs"
                    disabled={rowActionId === payment.id || !payment.autoRenewEnabled}
                    onClick={() => void testAutoCharge(payment)}
                  >
                    {rowActionId === payment.id ? "Выполняем…" : "Тест списания"}
                  </button>
                  <button
                    type="button"
                    className="ms-button ms-button-ghost ms-button-xs"
                    disabled={rowActionId === payment.id || payment.subscriptionStatus === "canceled"}
                    onClick={() => void revokeSubscription(payment)}
                  >
                    Снять доступ
                  </button>
                </div>
              </div>
              <div style={{ fontSize: 13, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>
                {payment.amount} {payment.currency}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
