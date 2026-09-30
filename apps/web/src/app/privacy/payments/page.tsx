'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2, legalH2Mt, legalUl, legalLink } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function PaymentsPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell contentKey="privacy.doc.payments" title={locale === 'en' ? t.docs.payments.title : 'Правила оплаты, подписки и возврата'}>
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600 }}>
        1. Способы оплаты
      </h2>
      <p>
        Оплата принимается через ЮKassa (банковские карты, СБП и другие способы,
        доступные на платёжной странице). Платёж проводится в рублях РФ.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        2. Тарифы и подписки
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Стоимость и состав тарифов указаны на странице <a href="/access" style={{ color: 'var(--accent-strong)' }}>/access</a> на момент покупки.</li>
        <li>Подписки могут быть с месячным или годовым периодом.</li>
        <li>Лимиты на сообщения действуют ежедневно и сбрасываются в 00:00 по серверному времени.</li>
        <li>Изменение тарифа в течение оплаченного периода — без перерасчёта.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        3. Продление и отмена
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Подписка не продлевается автоматически.</li>
        <li>Для продолжения доступа Пользователь оформляет следующую оплату самостоятельно.</li>
        <li>Отменить подписку — просто не платить за следующий период. Доступ сохраняется до конца текущего оплаченного срока.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        4. Возврат средств
      </h2>
      <p>
        В соответствии со ст. 26.1 закона «О защите прав потребителей» Пользователь
        вправе отказаться от оплаченной услуги:
      </p>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>В течение 14 дней с момента покупки, если сервис не использовался (нет отправленных сообщений в платных режимах).</li>
        <li>Возврат — пропорциональный (за неиспользованный период) при технической невозможности оказания услуги по вине Оператора.</li>
      </ul>
      <p>
        Запрос на возврат отправляйте на
        <a href={`mailto:${OPERATOR.email}`} style={{ color: 'var(--accent-strong)' }}> {OPERATOR.email}</a> с указанием
        email аккаунта и даты платежа. Срок рассмотрения — до 10 рабочих дней.
        Возврат — на тот же способ оплаты.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        5. Чеки и налоги
      </h2>
      <p>
        Оператор является самозанятым (НПД). Чек об оплате формируется в приложении
        «Мой налог» и направляется на email аккаунта по запросу.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <h2 style={legalH2}>1. Payment methods</h2>
      <p>
        Payments are accepted via YooKassa (bank cards, the Faster Payments System, and
        other methods available on the payment page). Payments are processed in Russian
        rubles.
      </p>

      <h2 style={legalH2Mt}>2. Plans and subscriptions</h2>
      <ul style={legalUl}>
        <li>The price and contents of plans are listed on the <a href="/access" style={legalLink}>/access</a> page at the time of purchase.</li>
        <li>Subscriptions can be monthly or yearly.</li>
        <li>Message limits apply daily and reset at 00:00 server time.</li>
        <li>Changing your plan during a paid period — no recalculation.</li>
      </ul>

      <h2 style={legalH2Mt}>3. Renewal and cancellation</h2>
      <ul style={legalUl}>
        <li>Subscriptions do not renew automatically.</li>
        <li>To continue access, the User makes the next payment themselves.</li>
        <li>To cancel a subscription, simply don&apos;t pay for the next period. Access remains until the end of the current paid term.</li>
      </ul>

      <h2 style={legalH2Mt}>4. Refunds</h2>
      <p>
        In accordance with Article 26.1 of the Law on Consumer Rights Protection, the
        User has the right to withdraw from a paid service:
      </p>
      <ul style={legalUl}>
        <li>Within 14 days of purchase, if the service has not been used (no messages sent in paid modes).</li>
        <li>A proportional refund (for the unused period) if the service cannot technically be provided due to the Operator&apos;s fault.</li>
      </ul>
      <p>
        Send refund requests to
        <a href={`mailto:${OPERATOR.email}`} style={legalLink}> {OPERATOR.email}</a> with the
        account email and payment date. Processing time is up to 10 business days.
        Refunds go to the original payment method.
      </p>

      <h2 style={legalH2Mt}>5. Receipts and taxes</h2>
      <p>
        The Operator is self-employed (NPD taxpayer). A payment receipt is generated in
        the &laquo;Мой налог&raquo; (My Tax) app and sent to the account email on
        request.
      </p>
    </>
  )
}
