"use client";
import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import {
  DEFAULT_SHOP_BENEFITS,
  DEFAULT_SHOP_LIFECYCLE_STATUSES,
  DEFAULT_SHOP_PAYMENT_NOTES,
  DEFAULT_SHOP_PERIODS,
  DEFAULT_SHOP_TABS,
  DEFAULT_SHOP_TARIFF_GROUPS,
  type ShopBenefitContent,
  type ShopLifecycleStatusContent,
  type ShopPaymentNoteContent,
  type ShopPeriodContent,
  type ShopTabContent,
  type ShopTariffGroupContent,
} from "@/lib/shopContent";
import { clearSiteContentCache } from "@/lib/siteContent";
import {
  DEFAULT_LEGAL_DOCS,
  legalDocContentKey,
  type LegalDocTile,
} from "@/lib/legalDocs";
import { useAutoSave, StatusBadge } from "@/lib/useAutoSave";
import type {
  SubTab,
  ContentItem,
  Tile,
  FeatureItem,
  ComparisonItem,
  EditableListField,
} from "./_content/_types";
import {
  DEFAULT_ABOUT_FEATURES,
  DEFAULT_ABOUT_COMPARISONS,
} from "./_content/_defaults";
import { btnIcon } from "./_content/_styles";
import { StringField } from "./_content/StringField";
import { BooleanField } from "./_content/BooleanField";
import { TilesEditor } from "./_content/TilesEditor";
import { EditableListEditor } from "./_content/EditableListEditor";
import { OPERATOR_FOOTER_RU, OPERATOR_TELEGRAM_URL } from "@/lib/operator"
export function AdminContentTab() {
  const [sub, setSub] = useState<SubTab>("landing");
  const [items, setItems] = useState<ContentItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const j = await apiFetch<{ items: ContentItem[] }>(
        "/api/admin/site-content",
      );
      setItems(j.items || []);
    } catch (e) {
      setError(String((e as Error).message || e));
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void load();
  }, []);
  const save = async (key: string, value: unknown) => {
    setError("");
    setNotice("");
    try {
      await apiFetch("/api/admin/site-content", {
        method: "POST",
        body: JSON.stringify({ key, value }),
      });
      clearSiteContentCache();
      void load();
      setTimeout(() => setNotice(""), 2000);
    } catch (e) {
      setError(String((e as Error).message || e));
    }
  };
  return (
    <div className="card ms-admin-card">
      <div className="ms-admin-card-head">
        <h2>Контент сайта</h2>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div style={{ fontSize: 12, color: "var(--muted)" }}>
            Меняется без пересборки. Кэш — 5 мин.
          </div>
          <button
            type="button"
            onClick={async () => {
              setError("");
              setNotice("");
              try {
                await apiFetch("/api/admin/site-content/flush", {
                  method: "POST",
                });
                clearSiteContentCache();
                setNotice("Кэш сброшен — изменения видны сразу");
                setTimeout(() => setNotice(""), 2500);
              } catch (e) {
                setError(String((e as Error).message || e));
              }
            }}
            style={{
              padding: "6px 12px",
              borderRadius: 8,
              border: "1px solid var(--line)",
              background: "var(--card)",
              color: "var(--foreground)",
              fontSize: 12,
              fontWeight: 500,
              cursor: "pointer",
              fontFamily: "inherit",
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
            }}
            title="Сбрасывает 5-минутный кэш — все правки применяются прямо сейчас."
          >
            ↻ Принудительно обновить
          </button>
        </div>
      </div>
      {error && (
        <div
          style={{
            padding: 10,
            borderRadius: 8,
            background: "var(--danger-bg)",
            color: "#b00020",
            marginBottom: 12,
            fontSize: 13,
          }}
        >
          {error}
        </div>
      )}
      {notice && (
        <div
          style={{
            padding: 10,
            borderRadius: 8,
            background: "color-mix(in oklab, #1D9E75 14%, var(--card))",
            color: "var(--accent-strong)",
            marginBottom: 12,
            fontSize: 13,
          }}
        >
          {notice}
        </div>
      )}
      <div
        style={{
          display: "flex",
          gap: 6,
          marginBottom: 16,
          borderBottom: "1px solid var(--line)",
        }}
      >
        {(
          [
            { id: "landing", label: "Главная" },
            { id: "about", label: "О сервисе" },
            { id: "privacy", label: "Документы" },
            { id: "access", label: "Доступ / Промокод" },
            { id: "chat", label: "Чат" },
            { id: "features", label: "Витрины" },
          ] as { id: SubTab; label: string }[]
        ).map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setSub(s.id)}
            style={{
              padding: "8px 14px",
              border: "none",
              background: "transparent",
              color: sub === s.id ? "var(--foreground)" : "var(--muted)",
              fontSize: 13,
              fontWeight: sub === s.id ? 600 : 400,
              cursor: "pointer",
              fontFamily: "inherit",
              borderBottom:
                sub === s.id ? "2px solid #1D9E75" : "2px solid transparent",
              marginBottom: -1,
            }}
          >
            {s.label}
          </button>
        ))}
      </div>
      {loading && <div style={{ color: "var(--muted)" }}>Загрузка…</div>}
      {!loading && sub === "landing" && (
        <>
          <StringField
            ckey="landing.hero.title"
            label="Заголовок главной"
            placeholder="Опишите задачу — Стратум сам выберет режим"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.hero.subtitle"
            label="Подзаголовок"
            placeholder="Пишите обычным языком: Стратум поймёт задачу, выберет режим и продолжит разбор."
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.hero.cta_label"
            label="Текст кнопки CTA"
            placeholder="Посмотреть, как это работает"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.hero.cta_href"
            label="Куда ведёт CTA (URL)"
            placeholder="/about"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.cta.primary_label"
            label="Текст основной кнопки CTA («Начать пользоваться»)"
            placeholder="Разобрать свою задачу"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.legal_link_label"
            label="Текст ссылки «Правовая информация»"
            placeholder="Правовая информация"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.footer.copyright"
            label="Подвал (copyright)"
            placeholder={OPERATOR_FOOTER_RU}
            items={items}
            onSave={save}
            multiline
          />
          <div
            style={{
              padding: "10px 12px",
              borderRadius: 8,
              background: "color-mix(in oklab, var(--accent) 8%, var(--card))",
              border: "1px solid var(--accent)",
              fontSize: 12,
              color: "var(--muted)",
            }}
          >
            💡 В любом тексте можно использовать переменные:{" "}
            <code>{`{{modes_count}}`}</code>, <code>{`{{paid_count}}`}</code>,{" "}
            <code>{`{{total_count}}`}</code> — подставится текущее число режимов
            из БД.
          </div>
        </>
      )}
      {!loading && sub === "about" && (
        <>
          <StringField
            ckey="about.intro.title"
            label="Главный заголовок страницы"
            placeholder="Не просто нейросеть — платформа профессиональных режимов"
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="about.intro.subtitle"
            label="Главный подзаголовок страницы"
            placeholder="Обычная нейросеть — универсальный инструмент без фокуса..."
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="about.stats.modes_label"
            label="Подпись счётчика «режимов»"
            placeholder="режимов"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.stats.experts_value"
            label="Значение счётчика «экспертов» (можно 100% / 5+)"
            placeholder="100%"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.stats.experts_label"
            label="Подпись счётчика «инструкции от экспертов»"
            placeholder="инструкции от экспертов"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.comparison.title"
            label="Заголовок таблицы сравнения"
            placeholder="Сравнение с обычной нейросетью"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.comparison.header_aspect"
            label="Заголовок колонки 1 («Аспект»)"
            placeholder="Аспект"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.comparison.header_regular"
            label="Заголовок колонки 2 («Обычная нейросеть»)"
            placeholder="Обычная нейросеть"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.comparison.header_stratum"
            label="Заголовок колонки 3 («СТРАТУМ»)"
            placeholder="СТРАТУМ"
            items={items}
            onSave={save}
          />
          <EditableListEditor<ComparisonItem>
            ckey="about.comparisons"
            label="Строки таблицы сравнения"
            items={items}
            onSave={save}
            fallback={DEFAULT_ABOUT_COMPARISONS}
            fields={[
              { key: "aspect", label: "Аспект" },
              { key: "regular", label: "Обычная нейросеть", multiline: true },
              { key: "stratum", label: "СТРАТУМ", multiline: true },
            ]}
            makeNew={(i) => ({
              aspect: `Новый аспект ${i}`,
              regular: "Описание обычной нейросети",
              stratum: "Описание СТРАТУМ",
            })}
          />
          <StringField
            ckey="about.features.title"
            label="Заголовок блока «Что внутри»"
            placeholder="Что внутри"
            items={items}
            onSave={save}
          />
          <EditableListEditor<FeatureItem>
            ckey="about.features"
            label="Квадратики блока «Что внутри»"
            items={items}
            onSave={save}
            fallback={DEFAULT_ABOUT_FEATURES}
            fields={[
              { key: "icon", label: "Иконка" },
              { key: "title", label: "Заголовок" },
              { key: "body", label: "Текст", multiline: true },
            ]}
            makeNew={(i) => ({
              icon: "✨",
              title: `Новый пункт ${i}`,
              body: "Описание...",
            })}
          />
          <StringField
            ckey="about.how.title"
            label="Заголовок блока «Как это работает»"
            placeholder="Как это работает"
            items={items}
            onSave={save}
          />
          <TilesEditor ckey="about.tiles" items={items} onSave={save} />
          <StringField
            ckey="about.hero.title"
            label="Заголовок CTA-блока"
            placeholder="Попробуйте прямо сейчас"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="about.hero.subtitle"
            label="Подпись CTA-блока"
            placeholder="Первый режим бесплатно — просто введите промокод или войдите через Яндекс."
            items={items}
            onSave={save}
            multiline
          />
        </>
      )}
      {!loading && sub === "privacy" && (
        <>
          <StringField
            ckey="privacy.hub.title"
            label="Заголовок страницы документов"
            placeholder="Документы сервиса"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="privacy.hub.subtitle"
            label="Описание страницы документов"
            placeholder="Полный список юридических документов Mindstrata. Согласия — отдельными документами, как требует закон."
            items={items}
            onSave={save}
            multiline
          />
          <EditableListEditor<LegalDocTile>
            ckey="privacy.docs"
            label="Плашки документов"
            items={items}
            onSave={save}
            fallback={DEFAULT_LEGAL_DOCS}
            fields={[
              { key: "slug", label: "URL slug" },
              { key: "title", label: "Заголовок" },
              { key: "hint", label: "Подзаголовок", multiline: true },
              { key: "badge", label: "Бейдж" },
            ]}
            makeNew={(i) => ({
              slug: `doc-${i}`,
              title: `Новый документ ${i}`,
              hint: "Краткое описание документа.",
            })}
          />
          {DEFAULT_LEGAL_DOCS.map((doc) => (
            <div
              key={doc.slug}
              style={{
                marginTop: 20,
                paddingTop: 16,
                borderTop: "1px solid var(--line)",
              }}
            >
              <h3 style={{ margin: "0 0 12px", fontSize: 15 }}>{doc.title}</h3>
              <StringField
                ckey={legalDocContentKey(doc.slug, "title")}
                label="Заголовок страницы документа"
                placeholder={doc.title}
                items={items}
                onSave={save}
              />
              <StringField
                ckey={legalDocContentKey(doc.slug, "subtitle")}
                label="Подзаголовок / пометка документа"
                placeholder={doc.badge || "Mindstrata"}
                items={items}
                onSave={save}
              />
              <StringField
                ckey={legalDocContentKey(doc.slug, "body")}
                label="Текст документа (поддерживает ## заголовки, - списки, [ссылки](url))"
                placeholder="Оставьте пустым, чтобы показывался текст из кода. Введите текст, чтобы заменить содержимое страницы."
                items={items}
                onSave={save}
                multiline
              />
            </div>
          ))}
        </>
      )}
      {!loading && sub === "access" && (
        <>
          <div
            style={{
              padding: "10px 12px",
              borderRadius: 8,
              background: "color-mix(in oklab, var(--accent) 8%, var(--card))",
              border: "1px solid var(--accent)",
              fontSize: 12,
              color: "var(--muted)",
              marginBottom: 16,
            }}
          >
            🔧 Блок «Получить промокод» на странице{" "}
            <code>/access?force=promo</code>. URL валидируется отдельно — не
            сломать формат случайным вводом.
          </div>
          <StringField
            ckey="access.promo.help_prefix"
            label="Текст перед ссылкой"
            placeholder="Тестовый доступ можно запросить здесь:"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="access.promo.help_link_label"
            label="Подпись ссылки"
            placeholder="написать в Telegram"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="access.promo.help_link_href"
            label="URL ссылки (https://... или t.me/...)"
            placeholder={OPERATOR_TELEGRAM_URL}
            items={items}
            onSave={save}
          />
          <StringField
            ckey="access.shop_button_label"
            label="Текст кнопки магазина"
            placeholder="Посмотреть готовые решения"
            items={items}
            onSave={save}
          />
        </>
      )}
      {!loading && sub === "chat" && (
        <>
          <div
            style={{
              padding: "10px 12px",
              borderRadius: 8,
              background: "color-mix(in oklab, var(--accent) 8%, var(--card))",
              border: "1px solid var(--accent)",
              fontSize: 12,
              color: "var(--muted)",
              marginBottom: 16,
            }}
          >
            Тексты показываются пользователю в чате перед окончанием дневного
            запаса сообщений. Используйте короткую помощь, без внутренних
            терминов.
          </div>
          <BooleanField
            ckey="chat.quota.warning_enabled"
            label="Показывать предупреждение перед окончанием сообщений"
            hint="Выключите, если нужно временно убрать предупреждение без релиза."
            items={items}
            onSave={save}
            fallback={true}
          />
          <StringField
            ckey="chat.quota.warning_threshold_remaining"
            label="Показывать, когда осталось сообщений"
            placeholder="3"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="chat.quota.warning_threshold_percent"
            label="Или когда осталось процентов от дневного запаса"
            placeholder="20"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="chat.quota.warning_title"
            label="Заголовок предупреждения"
            placeholder="Сегодня осталось мало сообщений"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="chat.quota.warning_body"
            label="Текст предупреждения"
            placeholder="Если вопрос важный, отправьте его одним сообщением. Осталось {{remaining}} из {{limit}}."
            items={items}
            onSave={save}
            multiline
          />
        </>
      )}
      {!loading && sub === "features" && (
        <>
          <div
            style={{
              padding: "10px 12px",
              borderRadius: 8,
              background: "color-mix(in oklab, var(--accent) 8%, var(--card))",
              border: "1px solid var(--accent)",
              fontSize: 12,
              color: "var(--muted)",
              marginBottom: 16,
            }}
          >
            Включение сразу показывает ссылки на главной и странице промокода
            для всех пользователей. Прямые URL остаются живыми, но выключенный
            раздел показывает закрытое состояние.
          </div>
          <BooleanField
            ckey="features.blog_enabled"
            label="Показывать блог"
            hint="Добавляет кнопку «Блог» на главную и страницу промокода, открывает публичный раздел /blog."
            items={items}
            onSave={save}
          />
          <BooleanField
            ckey="features.shop_enabled"
            label="Показывать магазин решений"
            hint="Добавляет кнопку «Магазин решений», открывает /shop с тарифами из API."
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.blog_button_label"
            label="Текст кнопки блога на главной"
            placeholder="Блог"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="landing.shop_button_label"
            label="Текст кнопки магазина на главной"
            placeholder="Магазин решений"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="blog.hero.title"
            label="Заголовок блога"
            placeholder="Блог Mindstrata"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="blog.hero.subtitle"
            label="Подзаголовок блога"
            placeholder="Короткие разборы методик, выступлений и сценариев применения ИИ."
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="shop.hero.title"
            label="Заголовок магазина"
            placeholder="Магазин решений"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.hero.subtitle"
            label="Подзаголовок магазина"
            placeholder="Выберите срок доступа. В чате Стратум сам выберет режим под вашу задачу."
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="shop.loading_text"
            label="Текст загрузки тарифов"
            placeholder="Загрузка тарифов..."
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.empty_state"
            label="Пустое состояние магазина"
            placeholder="Сейчас нет активных публичных решений. В админке включите тарифы, доступные для подписки."
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="shop.closed.title"
            label="Заголовок закрытого магазина"
            placeholder="Магазин решений пока закрыт"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.closed.subtitle"
            label="Подпись закрытого магазина"
            placeholder="Раздел временно скрыт администратором."
            items={items}
            onSave={save}
            multiline
          />
          <StringField
            ckey="shop.tabs_label"
            label="ARIA-название вкладок магазина"
            placeholder="Разделы магазина решений"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.periods_label"
            label="ARIA-название выбора срока"
            placeholder="Срок подписки"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.buy_button_label"
            label="Текст кнопки покупки"
            placeholder="Купить решение"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.payment_loading_text"
            label="Текст кнопки при создании платежа"
            placeholder="Создаём платёж..."
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.autorenew_checkbox_label"
            label="Текст включённого автопродления"
            placeholder="Продлевать автоматически: {{period}}"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.one_time_checkbox_label"
            label="Текст разовой покупки"
            placeholder="Купить один период без автопродления"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.payment_hint"
            label="Короткая подпись под оплатой"
            placeholder="Доступ сразу после оплаты."
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.price_period_label"
            label="Подпись периода цены"
            placeholder="в месяц"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.total_price_template"
            label="Шаблон общей цены за период"
            placeholder="Итого за {{months}} мес.: {{total}}"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.daily_limit_template"
            label="Шаблон дневного лимита"
            placeholder="До {{count}} сообщений в день"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.solution_count_template"
            label="Шаблон количества решений"
            placeholder="Режимов внутри: {{count}}"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.included_items_label"
            label="Подпись списка включённых решений"
            placeholder="Режимы внутри"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.nav.chat_label"
            label="Текст кнопки «В чат» в магазине"
            placeholder="В чат"
            items={items}
            onSave={save}
          />
          <StringField
            ckey="shop.nav.login_label"
            label="Текст кнопки доступа в магазине"
            placeholder="Получить доступ"
            items={items}
            onSave={save}
          />
          <EditableListEditor<ShopTabContent>
            ckey="shop.tabs"
            label="Вкладки магазина"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_TABS}
            fields={[
              { key: "label", label: "Название вкладки" },
              { key: "groupName", label: "Группа тарифов из API (* = все)" },
              { key: "badge", label: "Бейдж вкладки" },
              {
                key: "description",
                label: "Описание вкладки",
                multiline: true,
              },
              { key: "enabled", label: "Включена: true / false" },
            ]}
            makeNew={(i) => ({
              label: `Новая вкладка ${i}`,
              groupName: `Группа ${i}`,
              description: "Короткое описание раздела магазина.",
              badge: "",
              enabled: true,
            })}
          />
          <EditableListEditor<ShopPeriodContent>
            ckey="shop.periods"
            label="Сроки покупки"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_PERIODS}
            fields={[
              { key: "id", label: "ID срока" },
              { key: "label", label: "Название кнопки" },
              { key: "months", label: "Месяцев для checkout" },
              { key: "discountPercent", label: "Скидка, %" },
              { key: "badge", label: "Бейдж / подсказка" },
              { key: "enabled", label: "Включён: true / false" },
            ]}
            makeNew={(i) => ({
              id: `${i}m`,
              label: `${i} мес.`,
              months: i,
              discountPercent: 0,
              badge: "",
              enabled: true,
            })}
          />
          <EditableListEditor<ShopTariffGroupContent>
            ckey="shop.tariff_groups"
            label="Тарифные группы магазина"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_TARIFF_GROUPS}
            fields={[
              { key: "groupName", label: "Группа тарифов из API (* = все)" },
              { key: "title", label: "Публичное название группы" },
              { key: "badge", label: "Бейдж группы" },
              { key: "description", label: "Описание группы", multiline: true },
              { key: "enabled", label: "Включена: true / false" },
            ]}
            makeNew={(i) => ({
              groupName: `Группа ${i}`,
              title: `Решения ${i}`,
              description: "Для какой ситуации подходит эта группа.",
              badge: "",
              enabled: true,
            })}
          />
          <EditableListEditor<ShopBenefitContent>
            ckey="shop.benefits"
            label="Преимущества решений"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_BENEFITS}
            fields={[
              { key: "title", label: "Заголовок" },
              { key: "body", label: "Описание", multiline: true },
              { key: "groupName", label: "Группа тарифов (* = все)" },
              {
                key: "tariffName",
                label: "Название тарифа (* или пусто = все)",
              },
              { key: "enabled", label: "Включено: true / false" },
            ]}
            makeNew={(i) => ({
              title: `Преимущество ${i}`,
              body: "Что пользователь получает после покупки.",
              groupName: "*",
              tariffName: "",
              enabled: true,
            })}
          />
          <EditableListEditor<ShopPaymentNoteContent>
            ckey="shop.payment_notes"
            label="Пояснения про оплату"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_PAYMENT_NOTES}
            fields={[
              { key: "title", label: "Короткий заголовок" },
              { key: "body", label: "Текст пояснения", multiline: true },
              { key: "enabled", label: "Включено: true / false" },
            ]}
            makeNew={(i) => ({
              title: `Пояснение ${i}`,
              body: "Короткое условие оплаты или продления.",
              enabled: true,
            })}
          />
          <EditableListEditor<ShopLifecycleStatusContent>
            ckey="shop.lifecycle_statuses"
            label="Статусы жизненного цикла покупки"
            items={items}
            onSave={save}
            fallback={DEFAULT_SHOP_LIFECYCLE_STATUSES}
            fields={[
              { key: "status", label: "Системный код" },
              { key: "label", label: "Публичная подпись" },
              {
                key: "description",
                label: "Описание статуса",
                multiline: true,
              },
              { key: "enabled", label: "Включено: true / false" },
            ]}
            makeNew={(i) => ({
              status: `status-${i}`,
              label: `Шаг ${i}`,
              description: "Что происходит на этом шаге.",
              enabled: true,
            })}
          />
        </>
      )}
    </div>
  );
}
