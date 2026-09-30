type BlogArticlePageProps = {
  params: Promise<{
    slug?: string
  }>
}

import { BlogArticlePageClient } from './BlogArticlePageClient'

export default async function BlogArticlePage({ params }: BlogArticlePageProps) {
  const resolvedParams = await params
  const slug = typeof resolvedParams.slug === 'string' ? decodeURIComponent(resolvedParams.slug) : ''
  return <BlogArticlePageClient slug={slug} />
}
