'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt, legalUl, legalLink } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function ConsentPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell
      contentKey="privacy.doc.consent"
      title={locale === 'en' ? t.docs.consent.title : 'Согласие на обработку персональных данных'}
      subtitle={locale === 'en' ? t.docs.consent.subtitle : 'отдельный документ · 152-ФЗ'}
    >
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <p>
        Регистрируясь в сервисе Mindstrata, Пользователь даёт настоящее согласие на
        обработку своих персональных данных. Согласие подтверждается отдельным
        действием при регистрации — оно не является «скрытой галочкой» внутри других
        документов.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Оператор
      </h2>
      <p>
        Самозанятый {OPERATOR.nameRu}<br />
        ИНН {OPERATOR.inn}<br />
        Email: <a href={`mailto:${OPERATOR.email}`} style={{ color: 'var(--accent-strong)' }}>{OPERATOR.email}</a>
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Перечень данных
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Идентификатор Яндекс-аккаунта (или email + пароль).</li>
        <li>Имя / отображаемое имя.</li>
        <li>Адрес электронной почты.</li>
        <li>Содержимое диалогов с ИИ.</li>
        <li>Технические данные: IP-адрес (в хэшированном виде), user-agent, время запросов.</li>
        <li>История активаций промокодов и подписок.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Цели обработки
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Регистрация и идентификация в сервисе.</li>
        <li>Хранение диалогов и сессии.</li>
        <li>Учёт лимитов и платежей.</li>
        <li>Безопасность.</li>
        <li>Исполнение требований законодательства.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Действия и условия
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Сбор, запись, систематизация, накопление, хранение, уточнение, использование, передача (см. ниже), обезличивание, блокирование, удаление, уничтожение.</li>
        <li>Обработка — смешанная (автоматизированная и неавтоматизированная).</li>
        <li>Хранение — на серверах в РФ. Передача — только сторонам, указанным в «Политике обработки персональных данных».</li>
        <li>Согласие действует с момента регистрации до его отзыва Пользователем.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Отзыв согласия
      </h2>
      <p>
        Согласие можно отозвать в любой момент: см. документ
        «<a href="/privacy/withdraw" style={{ color: 'var(--accent-strong)' }}>Отозвать согласие / удалить данные</a>».
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <p>
        By registering with the Mindstrata service, the User gives this consent to the
        processing of their personal data. Consent is confirmed by a separate action
        during registration — it is not a &laquo;hidden checkbox&raquo; inside other
        documents.
      </p>

      <h2 style={legalH2Mt}>Operator</h2>
      <p>
        Self-employed individual {OPERATOR.nameEn}<br />
        Tax ID {OPERATOR.inn}<br />
        Email: <a href={`mailto:${OPERATOR.email}`} style={legalLink}>{OPERATOR.email}</a>
      </p>

      <h2 style={legalH2Mt}>Data covered</h2>
      <ul style={legalUl}>
        <li>Yandex account ID (or email + password).</li>
        <li>Name / display name.</li>
        <li>Email address.</li>
        <li>The content of dialogues with the AI.</li>
        <li>Technical data: IP address (hashed), user agent, request timestamps.</li>
        <li>History of promo code and subscription activations.</li>
      </ul>

      <h2 style={legalH2Mt}>Purposes of processing</h2>
      <ul style={legalUl}>
        <li>Registration and identification in the service.</li>
        <li>Storing dialogues and the session.</li>
        <li>Tracking limits and payments.</li>
        <li>Security.</li>
        <li>Complying with legal requirements.</li>
      </ul>

      <h2 style={legalH2Mt}>Actions and conditions</h2>
      <ul style={legalUl}>
        <li>Collection, recording, systematization, accumulation, storage, clarification, use, transfer (see below), depersonalization, blocking, deletion, destruction.</li>
        <li>Processing is mixed (automated and non-automated).</li>
        <li>Storage — on servers in the Russian Federation. Transfer — only to the parties listed in the &laquo;Personal Data Processing Policy&raquo;.</li>
        <li>Consent is valid from the moment of registration until withdrawn by the User.</li>
      </ul>

      <h2 style={legalH2Mt}>Withdrawing consent</h2>
      <p>
        You can withdraw your consent at any time: see the document
        &laquo;<a href="/privacy/withdraw" style={legalLink}>Withdraw Consent / Delete Data</a>&raquo;.
      </p>
    </>
  )
}
