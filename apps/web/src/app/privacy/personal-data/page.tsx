'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt, legalUl, legalLink } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function PersonalDataPolicyPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell
      contentKey="privacy.doc.personal-data"
      title={locale === 'en' ? t.docs.personalData.title : 'Политика обработки персональных данных'}
      subtitle={locale === 'en' ? t.docs.personalData.subtitle : '152-ФЗ'}
    >
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <p>
        Настоящая Политика описывает, как самозанятый {OPERATOR.nameRu}
        (ИНН {OPERATOR.inn}, далее — «Оператор») обрабатывает персональные данные
        пользователей сервиса Mindstrata в соответствии с Федеральным законом
        № 152-ФЗ «О персональных данных».
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        1. Состав данных
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Идентификатор Яндекс-аккаунта, имя, email — при входе через Яндекс ID.</li>
        <li>Email и хэш пароля — при регистрации через email.</li>
        <li>Содержимое диалогов с ИИ-моделями.</li>
        <li>Технические логи: IP-адрес (в хэшированном виде), user-agent, время запросов.</li>
        <li>Использование промокодов, тарифов, дневные счётчики сообщений.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        2. Цели обработки
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Идентификация и авторизация пользователя.</li>
        <li>Хранение истории диалогов и текущей сессии.</li>
        <li>Учёт лимитов и платежей.</li>
        <li>Безопасность: rate limit, защита от атак.</li>
        <li>Исполнение требований законодательства РФ.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        3. Правовое основание
      </h2>
      <p>
        Обработка осуществляется на основании согласия субъекта данных (отдельный
        документ — «Согласие на обработку персональных данных»), а также пункта 5
        части 1 статьи 6 152-ФЗ — для исполнения договора (оферты).
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        4. Хранение и защита
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Данные хранятся на серверах, расположенных в Российской Федерации.</li>
        <li>Передача данных идёт по HTTPS/TLS.</li>
        <li>Пароли хранятся в виде хэша (PBKDF2-SHA256).</li>
        <li>Доступ к данным имеет только Оператор.</li>
        <li>Технические логи хранятся не более 90 дней.</li>
        <li>Диалоги — бессрочно до удаления аккаунта.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        5. Передача третьим лицам
      </h2>
      <p>Данные могут передаваться:</p>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Яндекс LLC — при OAuth-авторизации (проверка идентичности).</li>
        <li>Провайдерам ИИ-моделей (VseGPT / OpenAI-совместимые шлюзы) — содержимое диалогов передаётся для генерации ответа без привязки к идентифицирующим данным.</li>
        <li>По официальным запросам уполномоченных органов РФ.</li>
      </ul>
      <p>
        Оператор не передаёт данные рекламным сетям, брокерам и третьим лицам в
        маркетинговых целях.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        6. Права субъекта
      </h2>
      <p>В соответствии со статьями 14–17 152-ФЗ субъект данных вправе:</p>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Получить информацию об обработке своих данных.</li>
        <li>Требовать уточнения, блокирования или уничтожения.</li>
        <li>Отозвать согласие на обработку.</li>
      </ul>
      <p>
        Обращения направляйте на <a href={`mailto:${OPERATOR.email}`} style={{ color: 'var(--accent-strong)' }}>{OPERATOR.email}</a>.
        Срок ответа — не более 30 дней.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <p>
        This Policy describes how self-employed individual {OPERATOR.nameEn}
        (Tax ID {OPERATOR.inn}, hereinafter the &laquo;Operator&raquo;)
        processes the personal data of Mindstrata users in accordance with Federal Law
        No. 152-FZ &laquo;On Personal Data&raquo;.
      </p>

      <h2 style={legalH2Mt}>1. Data we collect</h2>
      <ul style={legalUl}>
        <li>Yandex account ID, name, email — when signing in via Yandex ID.</li>
        <li>Email and password hash — when registering via email.</li>
        <li>The content of dialogues with AI models.</li>
        <li>Technical logs: IP address (hashed), user agent, request timestamps.</li>
        <li>Promo code usage, subscription plans, daily message counters.</li>
      </ul>

      <h2 style={legalH2Mt}>2. Purposes of processing</h2>
      <ul style={legalUl}>
        <li>Identifying and authorizing the user.</li>
        <li>Storing dialogue history and the current session.</li>
        <li>Tracking limits and payments.</li>
        <li>Security: rate limiting, attack protection.</li>
        <li>Complying with Russian legal requirements.</li>
      </ul>

      <h2 style={legalH2Mt}>3. Legal basis</h2>
      <p>
        Processing is carried out on the basis of the data subject&apos;s consent (a
        separate document — &laquo;Consent to Personal Data Processing&raquo;), as well
        as clause 5, part 1, article 6 of 152-FZ — for performance of the contract
        (offer).
      </p>

      <h2 style={legalH2Mt}>4. Storage and protection</h2>
      <ul style={legalUl}>
        <li>Data is stored on servers located in the Russian Federation.</li>
        <li>Data is transmitted over HTTPS/TLS.</li>
        <li>Passwords are stored as hashes (PBKDF2-SHA256).</li>
        <li>Only the Operator has access to the data.</li>
        <li>Technical logs are kept for no more than 90 days.</li>
        <li>Dialogues are kept indefinitely until the account is deleted.</li>
      </ul>

      <h2 style={legalH2Mt}>5. Disclosure to third parties</h2>
      <p>Data may be shared with:</p>
      <ul style={legalUl}>
        <li>Yandex LLC — during OAuth authorization (identity verification).</li>
        <li>AI model providers (VseGPT / OpenAI-compatible gateways) — dialogue content is sent to generate responses without being linked to identifying data.</li>
        <li>Upon official requests from authorized Russian government bodies.</li>
      </ul>
      <p>
        The Operator does not share data with advertising networks, brokers, or third
        parties for marketing purposes.
      </p>

      <h2 style={legalH2Mt}>6. Data subject rights</h2>
      <p>In accordance with articles 14–17 of 152-FZ, the data subject has the right to:</p>
      <ul style={legalUl}>
        <li>Receive information about the processing of their data.</li>
        <li>Request correction, blocking, or destruction.</li>
        <li>Withdraw consent to processing.</li>
      </ul>
      <p>
        Send requests to <a href={`mailto:${OPERATOR.email}`} style={legalLink}>{OPERATOR.email}</a>.
        Response time is no more than 30 days.
      </p>
    </>
  )
}
