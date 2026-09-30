'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt } from '../_parts/_styles'

export default function CookiesPolicyPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell contentKey="privacy.doc.cookies" title={locale === 'en' ? t.docs.cookies.title : 'Политика cookies'}>
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <p>
        Mindstrata использует cookies только для технической работы сервиса. Аналитики,
        рекламы, трекеров и пикселей у нас нет.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Что мы делаем
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Используем только обязательные (технически необходимые) cookies.</li>
        <li>Они нужны для входа, сохранения сессии и безопасности.</li>
        <li>Мы не используем cookies для рекламы, аналитики и трекинга.</li>
        <li>В cookies не храним ФИО, email, пароль, содержимое промптов.</li>
        <li>Храним только случайный токен/идентификатор сессии.</li>
        <li>При выходе из аккаунта сессия сбрасывается.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Баннер согласия
      </h2>
      <p>
        Технические cookies (вход, сессия, безопасность) работают по умолчанию — иначе
        авторизация была бы невозможна. Кнопка «Принять» в баннере подтверждает ваше
        согласие на возможные расширенные функции в будущем (например, статистика
        использования режимов). Молчание согласием не считается: без явного клика
        расширенные сценарии не активируются.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Как отключить
      </h2>
      <p>
        Cookies можно отключить в настройках браузера. После этого вход в сервис
        работать не будет — это техническое требование.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <p>
        Mindstrata uses cookies only for the technical operation of the service. We have
        no analytics, advertising, trackers, or pixels.
      </p>

      <h2 style={legalH2Mt}>What we do</h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>We use only essential (technically necessary) cookies.</li>
        <li>They are needed for sign-in, session persistence, and security.</li>
        <li>We do not use cookies for advertising, analytics, or tracking.</li>
        <li>We do not store names, emails, passwords, or prompt content in cookies.</li>
        <li>We only store a random session token/identifier.</li>
        <li>The session is reset when you sign out.</li>
      </ul>

      <h2 style={legalH2Mt}>Consent banner</h2>
      <p>
        Technical cookies (sign-in, session, security) work by default — otherwise
        authorization would be impossible. The &laquo;Accept&raquo; button in the banner
        confirms your consent to possible extended features in the future (e.g. mode
        usage statistics). Silence is not considered consent: without an explicit click,
        extended scenarios are not activated.
      </p>

      <h2 style={legalH2Mt}>How to disable</h2>
      <p>
        Cookies can be disabled in your browser settings. After that, signing in to the
        service will not work — this is a technical requirement.
      </p>
    </>
  )
}
