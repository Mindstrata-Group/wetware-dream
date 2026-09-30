import { describe, expect, it } from 'vitest'

import { blogPostCoverUrl, blogPostIsVisible, getPublishedBlogPosts, type BlogPostContent } from './blogContent'

const basePost: BlogPostContent = {
  slug: 'case',
  title: 'Кейс',
  excerpt: 'Коротко',
  tag: 'Кейсы',
  author: 'Редакция',
  date: '2026-06-05T10:00:00Z',
  body: 'Текст',
  status: 'published',
}

describe('blogContent', () => {
  it('treats scheduled posts as visible only after their publish date', () => {
    const scheduled: BlogPostContent = {
      ...basePost,
      status: 'scheduled',
      date: '2026-06-10T12:00:00Z',
    }

    expect(blogPostIsVisible(scheduled, new Date('2026-06-10T11:59:59Z'))).toBe(false)
    expect(blogPostIsVisible(scheduled, new Date('2026-06-10T12:00:00Z'))).toBe(true)
  })

  it('hides scheduled post with missing or invalid date', () => {
    expect(blogPostIsVisible({ ...basePost, status: 'scheduled', date: '' })).toBe(false)
    expect(blogPostIsVisible({ ...basePost, status: 'scheduled', date: 'not-a-date' })).toBe(false)
  })

  it('hides posts with unknown status', () => {
    // kills the mutation return status === 'published' -> return true
    expect(blogPostIsVisible({ ...basePost, status: 'archived' as any })).toBe(false)
    expect(blogPostIsVisible({ ...basePost, status: 'removed' as any })).toBe(false)
  })

  it('treats empty status as published (убивает || "published" → || "" мутацию)', () => {
    expect(blogPostIsVisible({ ...basePost, status: '' as any })).toBe(true)
  })

  it('falls back to DEFAULT_BLOG_POSTS when no blog.posts key in content', () => {
    // kills the mutation in getPublishedBlogPosts/getBlogPosts: the getArray fallback
    const posts = getPublishedBlogPosts({})
    expect(posts.length).toBeGreaterThan(0)
    expect(posts[0].slug).toBeTruthy()
  })

  it('keeps draft and future scheduled posts out of the public list', () => {
    const posts = getPublishedBlogPosts({
      'blog.posts': [
        { ...basePost, slug: 'published', title: 'Опубликовано', status: 'published', date: '2026-06-05T10:00:00Z' },
        { ...basePost, slug: 'draft', title: 'Черновик', status: 'draft', date: '2026-06-05T10:00:00Z' },
        { ...basePost, slug: 'future', title: 'Будущий пост', status: 'scheduled', date: '2099-01-01T00:00:00Z' },
      ],
    })

    expect(posts.map((post) => post.slug)).toEqual(['published'])
  })

  it('sorts pinned posts before non-pinned, then by date descending', () => {
    // kills mutations in sort: the pinned branches and the localeCompare direction
    const posts = getPublishedBlogPosts({
      'blog.posts': [
        { ...basePost, slug: 'old', date: '2026-01-01T00:00:00Z', status: 'published' },
        { ...basePost, slug: 'new', date: '2026-06-05T00:00:00Z', status: 'published' },
        { ...basePost, slug: 'pinned-old', date: '2025-12-01T00:00:00Z', status: 'published', pinned: true },
        { ...basePost, slug: 'pinned-new', date: '2026-05-01T00:00:00Z', status: 'published', pinned: true },
      ],
    })

    expect(posts.map(p => p.slug)).toEqual(['pinned-new', 'pinned-old', 'new', 'old'])
  })

  it('extracts cover URL from either legacy string or uploaded media object', () => {
    expect(blogPostCoverUrl({ ...basePost, cover: '/legacy-cover.png' })).toBe('/legacy-cover.png')
    expect(blogPostCoverUrl({ ...basePost, cover: { name: 'cover.png', url: '/api/public/site-media/1/cover.png' } })).toBe('/api/public/site-media/1/cover.png')
    expect(blogPostCoverUrl({ ...basePost, cover: { name: 'broken.png' } })).toBe('')
    expect(blogPostCoverUrl({ ...basePost, cover: null })).toBe('')
    expect(blogPostCoverUrl({ ...basePost })).toBe('')
  })
})
