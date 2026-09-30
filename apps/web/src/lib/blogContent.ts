import { getArray, type SiteContent } from './siteContent'
import type { Locale } from './i18n/types'

export type BlogPostStatus = 'published' | 'draft' | 'scheduled'
export type BlogPostLocale = 'ru' | 'en' | 'both'

export type BlogPostContent = {
  id?: number | string | null
  slug: string
  title: string
  excerpt: string
  tag: string
  author: string
  date: string
  updatedAt?: string
  body: string
  status: BlogPostStatus
  cover?: string | { name?: string; url?: string } | null
  featured?: boolean
  pinned?: boolean
  seoTitle?: string
  seoDesc?: string
  readMinutes?: string
  locale?: BlogPostLocale
}

export const DEFAULT_BLOG_POSTS: BlogPostContent[] = [
  {
    slug: 'speaker-qr-exercise',
    title: 'QR-упражнение на выступлении: как увидеть спрос в зале',
    excerpt: 'Участник получает личный ответ за несколько минут, а спикер видит, какие темы и офферы стоит усилить во второй части.',
    tag: 'Выступления',
    author: 'Mindstrata',
    date: '2026-06-05',
    body: 'Короткое ИИ-упражнение по QR-коду помогает собрать реальные вопросы зала до второй части выступления.',
    status: 'published',
    featured: true,
    pinned: true,
    readMinutes: '5 мин',
  },
  {
    slug: 'weekly-team-snapshot',
    title: 'Еженедельный срез ошибок продавцов и менеджеров',
    excerpt: 'Как короткие упражнения помогают владельцу микробизнеса замечать повторяющиеся потери без ручных разборов каждого разговора.',
    tag: 'Команды',
    author: 'Mindstrata',
    date: '2026-06-05',
    body: 'Сотрудники проходят упражнения по роли, а владелец получает повторяющиеся ошибки и риски по продажам и сервису.',
    status: 'published',
    featured: true,
    readMinutes: '6 мин',
  },
  {
    slug: 'psychologist-hard-cases',
    title: 'Тренажёр сложных случаев для психолога',
    excerpt: 'Безопасная практика между сессиями: типаж, протокол, неудобные вопросы и разбор профессионального решения.',
    tag: 'Психология',
    author: 'Mindstrata',
    date: '2026-06-05',
    body: 'Психолог задаёт случай, выбирает типаж и протокол, а система ведёт диалог в заданной модели.',
    status: 'published',
    featured: true,
    readMinutes: '7 мин',
  },
  {
    slug: 'real-estate-first-calls',
    title: 'Первые звонки новичка в агентстве недвижимости',
    excerpt: 'Сценарии торга, недоверия к комиссии и конфликтов по объекту до допуска к реальным клиентам.',
    tag: 'Продажи',
    author: 'Mindstrata',
    date: '2026-06-05',
    body: 'Новичок тренирует типовые сценарии агентства до допуска к реальным звонкам.',
    status: 'published',
    readMinutes: '5 мин',
  },
]

export function getBlogPosts(content: SiteContent): BlogPostContent[] {
  return getArray<BlogPostContent>(content, 'blog.posts', DEFAULT_BLOG_POSTS)
    .filter((post) => post && typeof post.title === 'string' && typeof post.slug === 'string')
}

export function blogPostCoverUrl(post: BlogPostContent): string {
  if (!post.cover) return ''
  if (typeof post.cover === 'string') return post.cover
  return typeof post.cover.url === 'string' ? post.cover.url : ''
}

export function blogPostIsVisible(post: BlogPostContent, now = new Date()): boolean {
  const status = post.status || 'published'
  if (status === 'draft') return false
  if (status === 'scheduled') {
    if (!post.date) return false
    const publishAt = new Date(post.date)
    return !Number.isNaN(publishAt.getTime()) && publishAt.getTime() <= now.getTime()
  }
  return status === 'published'
}

export function blogPostMatchesLocale(post: BlogPostContent, locale: Locale): boolean {
  const postLocale = post.locale || 'both'
  return postLocale === 'both' || postLocale === locale
}

export function getPublishedBlogPosts(content: SiteContent, locale: Locale = 'ru'): BlogPostContent[] {
  return getBlogPosts(content)
    .filter((post) => blogPostIsVisible(post) && blogPostMatchesLocale(post, locale))
    .sort((a, b) => {
      if (a.pinned !== b.pinned) return a.pinned ? -1 : 1
      return String(b.date || '').localeCompare(String(a.date || ''))
    })
}
