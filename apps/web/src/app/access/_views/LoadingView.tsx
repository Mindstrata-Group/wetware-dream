'use client'

import { MS } from '../_parts/_theme'
import { useT } from '@/lib/i18n/LocaleProvider'
import { accessText } from '../translations'

export function LoadingView() {
  const t = useT(accessText)
  return (
    <main
      style={{
        position: 'relative',
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '40px 20px',
        color: MS.ink50,
      }}
    >
      {t.loading.checkingAccess}
    </main>
  )
}
