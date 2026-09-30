'use client'

import { useEffect, useState } from 'react'
import { useApi } from './useApi'
import { type SiteContent } from './siteContent'

type SiteContentResponse = {
  content?: SiteContent
}

// Hook that gives content to a component. The first render returns {} (or the previous cache),
// then re-renders with real data after fetch.
//
// On the home page 1 fetch for the whole page, 60 s browser cache.
export function useSiteContent(): SiteContent {
  const { data } = useApi<SiteContentResponse>('/api/public/site-content', { cacheTtlMs: 60_000 })
  const [content, setContent] = useState<SiteContent>({})

  useEffect(() => {
    if (data?.content) setContent(data.content)
  }, [data])

  return content
}
