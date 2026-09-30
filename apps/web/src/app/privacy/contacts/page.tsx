'use client'

import { LegalDocShell } from '@/components/LegalDocShell'
import { useLocale, useT } from '@/lib/i18n/LocaleProvider'
import { privacyText } from '../translations'
import { legalH2Mt, legalLink } from '../_parts/_styles'
import { OPERATOR } from '@/lib/operator'

export default function ContactsPage() {
  const locale = useLocale()
  const t = useT(privacyText)
  return (
    <LegalDocShell contentKey="privacy.doc.contacts" title={locale === 'en' ? t.docs.contacts.title : 'Реквизиты и контакты'}>
      {locale === 'en' ? <EnContent /> : <RuContent />}
    </LegalDocShell>
  )
}

function RuContent() {
  return (
    <>
      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600 }}>
        Оператор сервиса
      </h2>
      <p>
        <strong>{OPERATOR.nameRu}</strong><br />
        Самозанятый (плательщик НПД)<br />
        ИНН: <strong>{OPERATOR.inn}</strong>
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Связь
      </h2>
      <p>
        Email для любых обращений (поддержка, юридические запросы, возврат средств,
        отзыв согласия): <a href={`mailto:${OPERATOR.email}`} style={{ color: 'var(--accent-strong)' }}>{OPERATOR.email}</a>
      </p>
      <p>
        Срок ответа — до 30 дней (по запросам, связанным с обработкой персональных
        данных — до 10 рабочих дней).
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Сервис
      </h2>
      <p>
        Сайт: <a href={OPERATOR.siteUrl} style={{ color: 'var(--accent-strong)' }}>{OPERATOR.siteLabel}</a>
      </p>

      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600, marginTop: 32 }}>
        Платёжная информация
      </h2>
      <p>
        Платежи принимаются через ЮKassa. Чеки формируются в приложении «Мой налог»
        и доступны по запросу на тот же email.
      </p>
    </>
  )
}

function EnContent() {
  return (
    <>
      <h2 style={{ fontFamily: '"Unbounded", system-ui, sans-serif', fontSize: 18, fontWeight: 600 }}>
        Service operator
      </h2>
      <p>
        <strong>{OPERATOR.nameEn}</strong><br />
        Self-employed (NPD taxpayer)<br />
        Tax ID: <strong>{OPERATOR.inn}</strong>
      </p>

      <h2 style={legalH2Mt}>Contact</h2>
      <p>
        Email for any inquiries (support, legal requests, refunds, withdrawal of
        consent): <a href={`mailto:${OPERATOR.email}`} style={legalLink}>{OPERATOR.email}</a>
      </p>
      <p>
        Response time — up to 30 days (for requests related to personal data
        processing — up to 10 business days).
      </p>

      <h2 style={legalH2Mt}>Service</h2>
      <p>
        Website: <a href={OPERATOR.siteUrl} style={legalLink}>{OPERATOR.siteLabel}</a>
      </p>

      <h2 style={legalH2Mt}>Payment information</h2>
      <p>
        Payments are accepted via YooKassa. Receipts are generated in the &laquo;Мой
        налог&raquo; (My Tax) app and available on request at the same email.
      </p>
    </>
  )
}
