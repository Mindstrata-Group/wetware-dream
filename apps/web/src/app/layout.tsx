import './globals.css'
import './fonts.css'
import type { Metadata, Viewport } from 'next'
import { ErrorReporterMount } from './_components/ErrorReporterMount'
import { MetrikaMount } from './_components/MetrikaMount'
import { LanguageSwitcher } from './_components/LanguageSwitcher'
import { getLocale } from '@/lib/i18n/server'
import { LocaleProvider } from '@/lib/i18n/LocaleProvider'

export const viewport: Viewport = {
  width: 'device-width',
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  // On mobile: the viewport resizes when the keyboard is shown; the chrome
  // (address bar, bottom menu) hides when body scrolls, giving more
  // screen to the chat.
  interactiveWidget: 'resizes-content',
  viewportFit: 'cover',
}

export async function generateMetadata(): Promise<Metadata> {
  const locale = await getLocale()
  return {
    title: locale === 'en' ? 'STRATUM Mindstrata' : 'СТРАТУМ Mindstrata',
    description:
      locale === 'en'
        ? 'Mindstrata platform: access to modes, chats and scenarios'
        : 'Платформа доступа к режимам, чатам и сценариям Mindstrata',
    icons: {
      icon: '/brand/logo-mark.png',
      shortcut: '/brand/logo-mark.png',
      apple: '/brand/logo-mark.png',
    },
  }
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale()
  return (
    <html lang={locale}>
      <head>
        {/* The theme is applied before render, without flicker */}
        <script dangerouslySetInnerHTML={{ __html: `
          try {
            var t = localStorage.getItem('ms_theme');
            var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
            if (t === 'dark' || t === 'light') {
              document.documentElement.dataset.theme = t;
            } else if (t === 'auto') {
              document.documentElement.dataset.theme = prefersDark ? 'dark' : 'light';
            } else if (prefersDark) {
              document.documentElement.dataset.theme = 'dark';
            }
          } catch(e) {}
        ` }} />
      </head>
      <body className="ms-body">
        <ErrorReporterMount />
        <MetrikaMount />
        <LocaleProvider locale={locale}>
          {children}
          <LanguageSwitcher />
        </LocaleProvider>
      </body>
    </html>
  )
}