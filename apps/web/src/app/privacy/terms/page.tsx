'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt, legalUl } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function TermsPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell contentKey="privacy.doc.terms" title={locale === 'en' ? t.docs.terms.title : 'Пользовательское соглашение / Оферта'}>
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <p>
        Настоящий документ является публичной офертой самозанятого {OPERATOR.nameRuGenitive}
        (ИНН {OPERATOR.inn}, далее — «Оператор») в адрес любого лица,
        принимающего её условия (далее — «Пользователь»), о предоставлении доступа к
        сервису Mindstrata (mindstrata.ru).
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        1. Предмет
      </h2>
      <p>
        Оператор предоставляет Пользователю доступ к веб-сервису для взаимодействия с
        ИИ-моделями через предопределённые «режимы». Доступ предоставляется на
        условиях, описанных на странице тарифов и в этой оферте.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        2. Принятие условий
      </h2>
      <p>
        Использование сервиса (регистрация, вход через Яндекс ID, отправка сообщений,
        активация промокода или оплата подписки) означает безоговорочное принятие
        настоящей оферты. Если вы не согласны — прекратите использование сервиса.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        3. Регистрация и доступ
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Регистрация выполняется через Яндекс ID или email + пароль.</li>
        <li>Пользователь обязуется указывать достоверные данные.</li>
        <li>Логин и пароль конфиденциальны и не подлежат передаче третьим лицам.</li>
        <li>Оператор вправе ограничить доступ при злоупотреблениях (см. п. 6).</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        4. Тарифы и лимиты
      </h2>
      <p>
        Доступ к платным режимам осуществляется в рамках активной подписки или
        промокода. Дневные лимиты сообщений действуют для каждого режима и указаны на
        странице доступов. Текущее использование отображается в личном кабинете.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        5. Содержимое и интеллектуальная собственность
      </h2>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Промпты режимов, дизайн и код сервиса принадлежат Оператору.</li>
        <li>Тексты диалогов Пользователя принадлежат ему; они хранятся в его аккаунте.</li>
        <li>Оператор не публикует содержимое диалогов без согласия Пользователя.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        6. Запреты
      </h2>
      <p>Запрещено использовать сервис для:</p>
      <ul style={{ paddingLeft: 20, margin: '12px 0' }}>
        <li>Создания и распространения незаконного контента.</li>
        <li>Реверс-инжиниринга, парсинга, автоматизированных атак.</li>
        <li>Распространения вредоносного ПО.</li>
        <li>Нарушения прав третьих лиц и законодательства РФ.</li>
      </ul>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        7. Ответственность
      </h2>
      <p>
        Сервис предоставляется «как есть». Оператор не отвечает за качество и точность
        ответов ИИ. Ответы — не медицинская, юридическая или финансовая консультация.
        Решения, принятые на их основе, остаются на ответственности Пользователя.
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        8. Изменения
      </h2>
      <p>
        Оператор вправе изменять условия. Существенные изменения публикуются на
        mindstrata.ru/privacy не менее чем за 7 дней до вступления в силу. Продолжение
        использования сервиса после публикации означает согласие с изменениями.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <p>
        This document is a public offer by self-employed individual {OPERATOR.nameEn}
        (Tax ID {OPERATOR.inn}, hereinafter the &laquo;Operator&raquo;) to any
        person who accepts its terms (hereinafter the &laquo;User&raquo;) to provide
        access to the Mindstrata service (mindstrata.ru).
      </p>

      <h2 style={legalH2Mt}>1. Subject</h2>
      <p>
        The Operator provides the User with access to a web service for interacting
        with AI models through predefined &laquo;modes&raquo;. Access is provided under
        the terms described on the pricing page and in this offer.
      </p>

      <h2 style={legalH2Mt}>2. Acceptance of terms</h2>
      <p>
        Using the service (registration, signing in via Yandex ID, sending messages,
        activating a promo code, or paying for a subscription) constitutes
        unconditional acceptance of this offer. If you do not agree, stop using the
        service.
      </p>

      <h2 style={legalH2Mt}>3. Registration and access</h2>
      <ul style={legalUl}>
        <li>Registration is done via Yandex ID or email + password.</li>
        <li>The User undertakes to provide accurate information.</li>
        <li>The login and password are confidential and must not be shared with third parties.</li>
        <li>The Operator may restrict access in case of abuse (see section 6).</li>
      </ul>

      <h2 style={legalH2Mt}>4. Plans and limits</h2>
      <p>
        Access to paid modes is provided within an active subscription or promo code.
        Daily message limits apply per mode and are listed on the access page. Current
        usage is shown in the personal account.
      </p>

      <h2 style={legalH2Mt}>5. Content and intellectual property</h2>
      <ul style={legalUl}>
        <li>The mode prompts, design, and code of the service belong to the Operator.</li>
        <li>The text of the User&apos;s dialogues belongs to the User; it is stored in their account.</li>
        <li>The Operator does not publish dialogue content without the User&apos;s consent.</li>
      </ul>

      <h2 style={legalH2Mt}>6. Prohibited uses</h2>
      <p>Using the service is prohibited for:</p>
      <ul style={legalUl}>
        <li>Creating and distributing illegal content.</li>
        <li>Reverse engineering, scraping, or automated attacks.</li>
        <li>Distributing malware.</li>
        <li>Violating third-party rights and the laws of the Russian Federation.</li>
      </ul>

      <h2 style={legalH2Mt}>7. Liability</h2>
      <p>
        The service is provided &laquo;as is&raquo;. The Operator is not responsible for
        the quality or accuracy of AI responses. Responses are not medical, legal, or
        financial advice. Decisions made based on them remain the User&apos;s
        responsibility.
      </p>

      <h2 style={legalH2Mt}>8. Changes</h2>
      <p>
        The Operator may change these terms. Material changes are published at
        mindstrata.ru/privacy at least 7 days before they take effect. Continuing to
        use the service after publication means you agree to the changes.
      </p>
    </>
  )
}
