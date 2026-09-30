import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react'

const apiFetchMock = vi.fn()
vi.mock('@/lib/api', () => ({
  apiBase: 'http://api.test',
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}))
vi.mock('@/lib/siteContent', async (orig) => {
  const actual = await orig<typeof import('@/lib/siteContent')>()
  return { ...actual, clearSiteContentCache: vi.fn() }
})
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

import { AdminBlogCmsTab } from './AdminBlogCmsTab'

const initialPosts = [
  {
    slug: 'first-post',
    title: 'Первая статья',
    excerpt: 'Описание',
    tag: 'Кейсы',
    author: 'Редакция',
    date: '2026-06-05',
    updatedAt: '2026-06-05T10:00:00Z',
    body: 'Текст',
    status: 'published',
    readMinutes: '4 мин',
  },
]

describe('AdminBlogCmsTab', () => {
  let loadedPosts = initialPosts

  beforeEach(() => {
    loadedPosts = initialPosts
    apiFetchMock.mockReset()
    apiFetchMock.mockImplementation(async (url: string, opts?: RequestInit) => {
      if (url === '/api/admin/site-content' && !opts) {
        return { items: [{ key: 'blog.posts', value: loadedPosts, updatedAt: '2026-06-05T10:00:00Z' }] }
      }
      return { ok: true }
    })
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({
      ok: true,
      url: '/api/public/site-media/1/cover.png',
      filename: 'cover.png',
    }), { status: 200, headers: { 'content-type': 'application/json' } })) as typeof fetch
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('показывает список постов и открывает редактор', async () => {
    render(<AdminBlogCmsTab />)

    expect(await screen.findByRole('heading', { name: 'Blog CMS' })).toBeTruthy()
    expect(await screen.findByText('Первая статья')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Править' }))
    expect(screen.getByDisplayValue('first-post')).toBeTruthy()
    expect(screen.getByDisplayValue('Первая статья')).toBeTruthy()
  })

  it('валидирует дублирующийся slug перед сохранением', async () => {
    render(<AdminBlogCmsTab />)
    await screen.findByText('Первая статья')

    fireEvent.click(screen.getByRole('button', { name: 'Новая статья' }))
    fireEvent.change(screen.getByPlaceholderText('Название статьи'), { target: { value: 'Дубль' } })
    fireEvent.change(screen.getByPlaceholderText('article-slug'), { target: { value: 'first-post' } })

    expect(screen.getByText('Такой slug уже есть')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Сохранить' })).toHaveProperty('disabled', true)
  })

  it('сохраняет новую статью в site_content', async () => {
    render(<AdminBlogCmsTab />)
    await screen.findByText('Первая статья')

    fireEvent.click(screen.getByRole('button', { name: 'Новая статья' }))
    fireEvent.change(screen.getByPlaceholderText('Название статьи'), { target: { value: 'New Product Case' } })
    fireEvent.change(screen.getByPlaceholderText('article-slug'), { target: { value: 'new-product-case' } })
    fireEvent.change(screen.getByPlaceholderText('2-3 строки для карточки'), { target: { value: 'Коротко' } })
    fireEvent.change(screen.getByPlaceholderText(/## Заголовок/), { target: { value: '## Раздел\n\nТекст' } })

    fireEvent.click(screen.getByRole('button', { name: 'Сохранить' }))

    await waitFor(() => {
      const postCall = apiFetchMock.mock.calls.find((call) => call[0] === '/api/admin/site-content' && call[1]?.method === 'POST')
      expect(postCall).toBeTruthy()
      const body = JSON.parse(String(postCall?.[1].body))
      expect(body.key).toBe('blog.posts')
      expect(body.value[0].slug).toBe('new-product-case')
    })
  })

  it('загружает картинку и вставляет markdown в тело', async () => {
    render(<AdminBlogCmsTab />)
    await screen.findByText('Первая статья')

    fireEvent.click(screen.getByRole('button', { name: 'Править' }))
    fireEvent.click(screen.getByRole('button', { name: 'Картинка' }))

    const fileInput = document.querySelector('input[type="file"][accept="image/*"]') as HTMLInputElement
    const file = new File(['png'], 'cover.png', { type: 'image/png' })
    fireEvent.change(fileInput, { target: { files: [file] } })

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('http://api.test/api/admin/site-media', expect.objectContaining({
        method: 'POST',
        credentials: 'include',
      }))
      expect(screen.getByDisplayValue(/!\[cover\.png\]\(\/api\/public\/site-media\/1\/cover\.png\)/)).toBeTruthy()
    })
  })

  it('показывает markdown-картинки, удаляет файл из базы и сохраняет статью без ссылки', async () => {
    loadedPosts = [{
      ...initialPosts[0],
      body: 'До\n\n![cover.png](/api/public/site-media/1/cover.png)\n\nПосле',
    }]
    render(<AdminBlogCmsTab />)
    await screen.findByText('Первая статья')

    fireEvent.click(screen.getByRole('button', { name: 'Править' }))
    await screen.findByText('Картинки в тексте')
    fireEvent.click(screen.getByRole('button', { name: 'Удалить из базы' }))

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith('http://api.test/api/admin/site-media?id=1', expect.objectContaining({
        method: 'DELETE',
        credentials: 'include',
      }))
      const postCall = apiFetchMock.mock.calls.find((call) => call[0] === '/api/admin/site-content' && call[1]?.method === 'POST')
      expect(postCall).toBeTruthy()
      const body = JSON.parse(String(postCall?.[1].body))
      expect(body.value[0].body).toBe('До\n\nПосле')
      expect(screen.queryByDisplayValue(/!\[cover\.png\]\(\/api\/public\/site-media\/1\/cover\.png\)/)).toBeNull()
    })
  })
})
