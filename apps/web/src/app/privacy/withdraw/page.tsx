'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt, legalUl, legalLink } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function WithdrawPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell contentKey="privacy.doc.withdraw" title={locale === 'en' ? t.docs.withdraw.title : 'Отозвать согласие / удалить данные'}>
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <p>
        Вы можете отозвать согласие на обработку персональных данных и потребовать
        удаления данных в любой момент. Никаких комиссий и санкций.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Способ 1: письмо на email
      </h2>
      <p>
        Отправьте письмо на
        <a href={`mailto:${OPERATOR.email}?subject=%D0%9E%D1%82%D0%B7%D1%8B%D0%B2%20%D1%81%D0%BE%D0%B3%D0%BB%D0%B0%D1%81%D0%B8%D1%8F%20%D0%BD%D0%B0%20%D0%BE%D0%B1%D1%80%D0%B0%D0%B1%D0%BE%D1%82%D0%BA%D1%83%20%D0%9F%D0%94`} style={{ color: 'var(--accent-strong)' }}> {OPERATOR.email}</a>
        с темой «Отзыв согласия на обработку ПД». В письме укажите:
      </p>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Email или Яндекс-логин, под которым зарегистрирован аккаунт.</li>
        <li>Что именно требуется: отозвать согласие, удалить данные, либо и то и другое.</li>
      </ul>
      <p>
        Срок рассмотрения и удаления — до 10 рабочих дней. По завершении вы получите
        ответ на тот же email.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Способ 2: кнопка в профиле
      </h2>
      <p>
        В разделе «Профиль» будет добавлена кнопка <em>«Удалить аккаунт»</em> с
        самостоятельным флоу. До момента добавления — пользуйтесь способом № 1.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Что будет удалено
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Учётная запись и привязки OAuth.</li>
        <li>История диалогов и сообщений.</li>
        <li>Активные сессии (вы будете разлогинены).</li>
        <li>Связи с использованными промокодами.</li>
      </ul>
      <p>
        Часть данных может сохраняться в обезличенной форме в технических логах и
        резервных копиях не более 90 дней — это обусловлено требованиями
        безопасности и резервного копирования. После этого — полное удаление.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Что произойдёт с подпиской
      </h2>
      <p>
        Если у вас активная подписка, удаление аккаунта прекращает доступ к сервису
        немедленно. Деньги за неиспользованный остаток оплаченного периода
        возвращаются по запросу — см. документ
        <a href="/privacy/payments" style={{ color: 'var(--accent-strong)' }}> «Правила оплаты, подписки и возврата»</a>.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <p>
        You can withdraw your consent to personal data processing and request data
        deletion at any time. No fees or penalties.
      </p>

      <h2 style={legalH2Mt}>Method 1: email</h2>
      <p>
        Send an email to
        <a href={`mailto:${OPERATOR.email}?subject=Withdrawal%20of%20personal%20data%20processing%20consent`} style={legalLink}> {OPERATOR.email}</a>
        with the subject &laquo;Withdrawal of personal data processing consent&raquo;.
        In the email, specify:
      </p>
      <ul style={legalUl}>
        <li>The email or Yandex login under which the account is registered.</li>
        <li>What exactly is requested: withdrawing consent, deleting data, or both.</li>
      </ul>
      <p>
        Processing and deletion time — up to 10 business days. You will receive a
        response at the same email once completed.
      </p>

      <h2 style={legalH2Mt}>Method 2: profile button</h2>
      <p>
        A &laquo;Delete account&raquo; button with a self-service flow will be added to
        the &laquo;Profile&raquo; section. Until then, use method 1.
      </p>

      <h2 style={legalH2Mt}>What will be deleted</h2>
      <ul style={legalUl}>
        <li>The account and OAuth connections.</li>
        <li>Dialogue and message history.</li>
        <li>Active sessions (you will be signed out).</li>
        <li>Links to used promo codes.</li>
      </ul>
      <p>
        Some data may remain in anonymized form in technical logs and backups for no
        more than 90 days — this is required for security and backup purposes. After
        that — full deletion.
      </p>

      <h2 style={legalH2Mt}>What happens to your subscription</h2>
      <p>
        If you have an active subscription, deleting your account terminates access to
        the service immediately. Money for the unused portion of the paid period is
        refunded on request — see the document
        <a href="/privacy/payments" style={legalLink}> &laquo;Payment, Subscription &amp; Refund Rules&raquo;</a>.
      </p>
    </>
  )
}
