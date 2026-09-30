'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import type React from 'react'
import Link from 'next/link'
import { EditableLegalBody } from '@/components/EditableLegalBody'
import { apiBase, apiFetch } from '@/lib/api'
import { blogPostCoverUrl, DEFAULT_BLOG_POSTS, type BlogPostContent, type BlogPostStatus } from '@/lib/blogContent'
import { clearSiteContentCache } from '@/lib/siteContent'
import type { ContentItem } from './_content/_types'
import { StringField } from './_content/StringField'
import { BooleanField } from './_content/BooleanField'

type BlogEditorTab = 'list' | 'edit' | 'preview' | 'settings'

const emptyPost = (): BlogPostContent => ({
  id: `draft-${Date.now()}`,
  slug: '',
  title: '',
  excerpt: '',
  tag: 'Методики',
  author: 'Mindstrata',
  date: new Date().toISOString().slice(0, 10),
  updatedAt: new Date().toISOString(),
  body: '',
  status: 'draft',
  cover: null,
  featured: false,
  pinned: false,
  readMinutes: '5 мин',
  seoTitle: '',
  seoDesc: '',
  locale: 'both',
})

export function AdminBlogCmsTab() {
  const [items, setItems] = useState<ContentItem[]>([])
  const [posts, setPosts] = useState<BlogPostContent[]>(DEFAULT_BLOG_POSTS)
  const [selectedSlug, setSelectedSlug] = useState('')
  const [draft, setDraft] = useState<BlogPostContent>(() => emptyPost())
  const [tab, setTab] = useState<BlogEditorTab>('list')
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | BlogPostStatus>('all')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deletingMediaId, setDeletingMediaId] = useState<number | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const bodyRef = useRef<HTMLTextAreaElement>(null)
  const coverInputRef = useRef<HTMLInputElement>(null)
  const bodyImageInputRef = useRef<HTMLInputElement>(null)

  async function load() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<{ items: ContentItem[] }>('/api/admin/site-content')
      const loadedItems = data.items || []
      setItems(loadedItems)
      const rawPosts = loadedItems.find((item) => item.key === 'blog.posts')?.value
      const nextPosts = Array.isArray(rawPosts) ? rawPosts as BlogPostContent[] : DEFAULT_BLOG_POSTS
      setPosts(nextPosts)
      if (!selectedSlug && nextPosts[0]?.slug) {
        setSelectedSlug(nextPosts[0].slug)
        setDraft(normalizePost(nextPosts[0]))
      }
    } catch (e) {
      setError(String((e as Error).message || e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const filteredPosts = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return posts.filter((post) => {
      const statusOk = statusFilter === 'all' || (post.status || 'published') === statusFilter
      if (!statusOk) return false
      if (!needle) return true
      return [post.title, post.slug, post.excerpt, post.tag, post.author].some((value) => String(value || '').toLowerCase().includes(needle))
    })
  }, [posts, query, statusFilter])

  const slugErrors = validateSlug(draft.slug, posts, selectedSlug)
  const canSave = !slugErrors.length && draft.title.trim() && draft.slug.trim()
  const bodyMediaUrls = useMemo(() => extractSiteMediaUrls(draft.body), [draft.body])

  async function saveContent(key: string, value: unknown) {
    await apiFetch('/api/admin/site-content', { method: 'POST', body: JSON.stringify({ key, value }) })
    clearSiteContentCache()
    await load()
  }

  async function savePost(nextStatus?: BlogPostStatus) {
    const normalized = normalizePost({ ...draft, status: nextStatus || draft.status, updatedAt: new Date().toISOString() })
    const errors = validateSlug(normalized.slug, posts, selectedSlug)
    if (errors.length) {
      setError(errors.join('. '))
      return
    }
    setSaving(true)
    setError('')
    setNotice('')
    try {
      const exists = posts.some((post) => post.slug === selectedSlug)
      const nextPosts = exists
        ? posts.map((post) => post.slug === selectedSlug ? normalized : post)
        : [normalized, ...posts]
      await apiFetch('/api/admin/site-content', {
        method: 'POST',
        body: JSON.stringify({ key: 'blog.posts', value: nextPosts }),
      })
      clearSiteContentCache()
      setPosts(nextPosts)
      setSelectedSlug(normalized.slug)
      setDraft(normalized)
      setNotice(nextStatus === 'published' ? 'Статья опубликована' : 'Статья сохранена')
      window.setTimeout(() => setNotice(''), 2500)
    } catch (e) {
      setError(String((e as Error).message || e))
    } finally {
      setSaving(false)
    }
  }

  async function archivePost(post: BlogPostContent) {
    const nextPosts = posts.map((item) => item.slug === post.slug ? normalizePost({ ...item, status: 'draft', updatedAt: new Date().toISOString() }) : item)
    await apiFetch('/api/admin/site-content', {
      method: 'POST',
      body: JSON.stringify({ key: 'blog.posts', value: nextPosts }),
    })
    clearSiteContentCache()
    setPosts(nextPosts)
    setNotice('Статья снята с публикации')
    window.setTimeout(() => setNotice(''), 2500)
  }

  function editPost(post: BlogPostContent) {
    setSelectedSlug(post.slug)
    setDraft(normalizePost(post))
    setTab('edit')
    setError('')
  }

  function createPost() {
    setSelectedSlug('')
    setDraft(emptyPost())
    setTab('edit')
    setError('')
  }

  function update<K extends keyof BlogPostContent>(key: K, value: BlogPostContent[K]) {
    setDraft((prev) => {
      const next = { ...prev, [key]: value }
      if (key === 'title' && !prev.slug.trim()) next.slug = slugify(String(value || ''))
      return next
    })
  }

  function insertBody(text: string) {
    const ta = bodyRef.current
    if (!ta) {
      update('body', `${draft.body}${text}`)
      return
    }
    const start = ta.selectionStart
    const end = ta.selectionEnd
    const next = draft.body.slice(0, start) + text + draft.body.slice(end)
    update('body', next)
    window.setTimeout(() => {
      ta.focus()
      ta.selectionStart = ta.selectionEnd = start + text.length
    }, 0)
  }

  async function uploadImage(file: File) {
    const form = new FormData()
    form.append('scope', 'blog')
    form.append('file', file)
    const res = await fetch(`${apiBase}/api/admin/site-media`, {
      method: 'POST',
      credentials: 'include',
      body: form,
    })
    const payload = await res.json().catch(() => ({}))
    if (!res.ok || payload.ok === false) throw new Error(payload.error || `HTTP ${res.status}`)
    return payload as { url: string; filename: string }
  }

  async function pickCover(file: File) {
    try {
      const uploaded = await uploadImage(file)
      update('cover', { name: uploaded.filename, url: uploaded.url })
    } catch (e) {
      setError(String((e as Error).message || e))
    }
  }

  async function insertBodyImage(file: File) {
    try {
      const uploaded = await uploadImage(file)
      insertBody(`\n\n![${uploaded.filename}](${uploaded.url})\n\n`)
    } catch (e) {
      setError(String((e as Error).message || e))
    }
  }

  function removeBodyMedia(url: string) {
    update('body', removeMediaMarkdown(draft.body, url))
  }

  async function persistMediaRemoval(nextDraft: BlogPostContent) {
    if (!selectedSlug) return
    const nextPost = normalizePost({ ...nextDraft, updatedAt: new Date().toISOString() })
    const nextPosts = posts.map((post) => post.slug === selectedSlug ? nextPost : post)
    await apiFetch('/api/admin/site-content', {
      method: 'POST',
      body: JSON.stringify({ key: 'blog.posts', value: nextPosts }),
    })
    clearSiteContentCache()
    setPosts(nextPosts)
    setDraft(nextPost)
    setSelectedSlug(nextPost.slug)
  }

  async function deleteSiteMedia(url: string) {
    const id = siteMediaIdFromUrl(url)
    if (!id) {
      setError('Не удалось определить id картинки')
      return
    }
    setDeletingMediaId(id)
    setError('')
    setNotice('')
    try {
      const res = await fetch(`${apiBase}/api/admin/site-media?id=${id}`, {
        method: 'DELETE',
        credentials: 'include',
      })
      const payload = await res.json().catch(() => ({}))
      if (!res.ok || payload.ok === false) throw new Error(payload.error || `HTTP ${res.status}`)
      const nextDraft = normalizePost({
        ...draft,
        cover: blogPostCoverUrl(draft) === url ? null : draft.cover,
        body: draft.body.includes(url) ? removeMediaMarkdown(draft.body, url) : draft.body,
      })
      await persistMediaRemoval(nextDraft)
      if (!selectedSlug) setDraft(nextDraft)
      setNotice('Картинка удалена из базы и убрана из статьи')
      window.setTimeout(() => setNotice(''), 2500)
    } catch (e) {
      setError(String((e as Error).message || e))
    } finally {
      setDeletingMediaId(null)
    }
  }

  return (
    <div className="card ms-admin-card">
      <div className="ms-admin-card-head">
        <div>
          <h2>Blog CMS</h2>
          <p style={hintStyle}>Отдельное управление статьями, обложками, публикацией и текстами блога без пересборки.</p>
        </div>
        <button type="button" className="ms-button ms-button-primary ms-button-xs" onClick={createPost}>Новая статья</button>
      </div>

      {error && <div className="ms-error-box" style={{ marginBottom: 12 }}>{error}</div>}
      {notice && <div className="ms-success-box" style={{ marginBottom: 12 }}>{notice}</div>}

      <div style={tabsStyle}>
        {[
          ['list', 'Статьи'],
          ['edit', selectedSlug ? 'Редактирование' : 'Новая статья'],
          ['preview', 'Preview'],
          ['settings', 'Настройки'],
        ].map(([id, label]) => (
          <button key={id} type="button" onClick={() => setTab(id as BlogEditorTab)} style={tab === id ? activeTabStyle : tabStyle}>{label}</button>
        ))}
      </div>

      {loading && <div className="ms-info-box">Загрузка…</div>}
      {!loading && tab === 'list' && (
        <section>
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', marginBottom: 14 }}>
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Поиск по статьям" style={inputStyle} />
            <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)} style={inputStyle}>
              <option value="all">Все статусы</option>
              <option value="published">Опубликованные</option>
              <option value="scheduled">Запланированные</option>
              <option value="draft">Черновики</option>
            </select>
          </div>
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 760, fontSize: 13 }}>
              <thead>
                <tr style={{ color: 'var(--muted)', textAlign: 'left' }}>
                  {['Статья', 'Slug', 'Тег', 'Дата', 'Обновлена', 'Статус', ''].map((head) => (
                    <th key={head} style={thStyle}>{head}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filteredPosts.map((post) => (
                  <tr key={post.slug} style={{ borderBottom: '1px solid var(--line)' }}>
                    <td style={tdStyle}>
                      <button type="button" onClick={() => editPost(post)} style={linkButtonStyle}>{post.title || 'Без названия'}</button>
                      <div style={{ color: 'var(--muted)', fontSize: 12, marginTop: 3 }}>{post.excerpt}</div>
                    </td>
                    <td style={{ ...tdStyle, fontFamily: 'ui-monospace, monospace', color: 'var(--muted)' }}>{post.slug}</td>
                    <td style={tdStyle}>{post.tag}</td>
                    <td style={tdStyle}>{post.date}</td>
                    <td style={tdStyle}>{formatDateTime(post.updatedAt)}</td>
                    <td style={tdStyle}><StatusPill status={post.status || 'published'} /></td>
                    <td style={tdStyle}>
                      <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => editPost(post)}>Править</button>
                        <Link className="ms-button ms-button-ghost ms-button-xs" href={`/blog/${encodeURIComponent(post.slug)}`} target="_blank">Открыть</Link>
                        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => void archivePost(post)}>В черновик</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {!loading && tab === 'edit' && (
        <section style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 300px', gap: 16 }} className="ms-blog-cms-editor">
          <div>
            <Field label="Заголовок">
              <input value={draft.title} onChange={(e) => update('title', e.target.value)} placeholder="Название статьи" style={inputStyle} />
            </Field>
            <Field label="Slug" hint={`/blog/${draft.slug || 'your-slug'}`}>
              <input value={draft.slug} onChange={(e) => update('slug', slugify(e.target.value))} placeholder="article-slug" style={{ ...inputStyle, fontFamily: 'ui-monospace, monospace' }} />
              {slugErrors.map((msg) => <div key={msg} style={{ color: '#b00020', fontSize: 12, marginTop: 4 }}>{msg}</div>)}
            </Field>
            <Field label="Краткое описание">
              <textarea value={draft.excerpt} onChange={(e) => update('excerpt', e.target.value)} rows={3} placeholder="2-3 строки для карточки" style={textareaStyle} />
            </Field>
            <Field label="Текст статьи / Markdown">
              <Toolbar onInsert={insertBody} onImage={() => bodyImageInputRef.current?.click()} />
              <textarea ref={bodyRef} value={draft.body} onChange={(e) => update('body', e.target.value)} rows={16} placeholder="## Заголовок&#10;&#10;Текст, таблицы, списки, изображения." style={{ ...textareaStyle, fontFamily: 'ui-monospace, monospace' }} />
              <input ref={bodyImageInputRef} type="file" accept="image/*" style={{ display: 'none' }} onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) void insertBodyImage(file)
                e.target.value = ''
              }} />
              {bodyMediaUrls.length > 0 && (
                <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
                  <div style={{ fontSize: 12, color: 'var(--muted)', fontWeight: 600 }}>Картинки в тексте</div>
                  {bodyMediaUrls.map((url) => {
                    const id = siteMediaIdFromUrl(url)
                    return (
                      <div key={url} style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) auto auto', gap: 6, alignItems: 'center', padding: '8px 10px', border: '1px solid var(--line)', borderRadius: 8, background: 'var(--soft)' }}>
                        <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 12, color: 'var(--muted)' }}>{url}</span>
                        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => removeBodyMedia(url)}>Убрать из текста</button>
                        <button type="button" className="ms-button ms-button-ghost ms-button-xs" disabled={!id || deletingMediaId === id} onClick={() => void deleteSiteMedia(url)}>
                          {deletingMediaId === id ? 'Удаляем…' : 'Удалить из базы'}
                        </button>
                      </div>
                    )
                  })}
                </div>
              )}
            </Field>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
              <button type="button" className="ms-button ms-button-primary ms-button-xs" disabled={saving || !canSave} onClick={() => void savePost()}>{saving ? 'Сохраняем…' : 'Сохранить'}</button>
              <button type="button" className="ms-button ms-button-ghost ms-button-xs" disabled={saving || !canSave} onClick={() => void savePost('published')}>Опубликовать</button>
              <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => setTab('preview')}>Preview</button>
            </div>
          </div>

          <aside style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div style={sideCardStyle}>
              <Field label="Статус">
                <select value={draft.status} onChange={(e) => update('status', e.target.value as BlogPostStatus)} style={inputStyle}>
                  <option value="draft">Черновик</option>
                  <option value="scheduled">Запланирован</option>
                  <option value="published">Опубликован</option>
                </select>
              </Field>
              <Field label="Дата публикации">
                <input type="date" value={String(draft.date || '').slice(0, 10)} onChange={(e) => update('date', e.target.value)} style={inputStyle} />
              </Field>
              <Field label="Дата обновления">
                <input value={formatDateTime(draft.updatedAt)} readOnly style={{ ...inputStyle, color: 'var(--muted)' }} />
              </Field>
            </div>
            <div style={sideCardStyle}>
              <Field label="Автор">
                <input value={draft.author} onChange={(e) => update('author', e.target.value)} style={inputStyle} />
              </Field>
              <Field label="Тег">
                <input value={draft.tag} onChange={(e) => update('tag', e.target.value)} style={inputStyle} />
              </Field>
              <Field label="Время чтения">
                <input value={draft.readMinutes || ''} onChange={(e) => update('readMinutes', e.target.value)} style={inputStyle} />
              </Field>
              <Field label="Язык страницы" hint="На каких версиях сайта показывать статью">
                <select value={draft.locale || 'both'} onChange={(e) => update('locale', e.target.value as BlogPostContent['locale'])} style={inputStyle}>
                  <option value="both">RU + EN</option>
                  <option value="ru">Только RU</option>
                  <option value="en">Только EN</option>
                </select>
              </Field>
              <label style={checkStyle}><input type="checkbox" checked={!!draft.pinned} onChange={(e) => update('pinned', e.target.checked)} /> Закрепить сверху</label>
              <label style={checkStyle}><input type="checkbox" checked={!!draft.featured} onChange={(e) => update('featured', e.target.checked)} /> Важная статья</label>
            </div>
            <div style={sideCardStyle}>
              <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--muted)', marginBottom: 8 }}>Обложка</div>
              <CoverDropzone cover={blogPostCoverUrl(draft)} onPick={() => coverInputRef.current?.click()} onDrop={(file) => void pickCover(file)} />
              <input ref={coverInputRef} type="file" accept="image/*" style={{ display: 'none' }} onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) void pickCover(file)
                e.target.value = ''
              }} />
              <input value={blogPostCoverUrl(draft)} onChange={(e) => update('cover', e.target.value)} placeholder="или URL картинки" style={{ ...inputStyle, marginTop: 8 }} />
              {blogPostCoverUrl(draft) && (
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 }}>
                  <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => update('cover', null)}>Убрать обложку</button>
                  {siteMediaIdFromUrl(blogPostCoverUrl(draft)) && (
                    <button type="button" className="ms-button ms-button-ghost ms-button-xs" disabled={deletingMediaId === siteMediaIdFromUrl(blogPostCoverUrl(draft))} onClick={() => void deleteSiteMedia(blogPostCoverUrl(draft))}>
                      {deletingMediaId === siteMediaIdFromUrl(blogPostCoverUrl(draft)) ? 'Удаляем…' : 'Удалить из базы'}
                    </button>
                  )}
                </div>
              )}
            </div>
            <div style={sideCardStyle}>
              <Field label="SEO title">
                <input value={draft.seoTitle || ''} onChange={(e) => update('seoTitle', e.target.value)} style={inputStyle} />
              </Field>
              <Field label="SEO description">
                <textarea value={draft.seoDesc || ''} onChange={(e) => update('seoDesc', e.target.value)} rows={3} style={textareaStyle} />
              </Field>
            </div>
          </aside>
        </section>
      )}

      {!loading && tab === 'preview' && (
        <article style={{ maxWidth: 760 }}>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            <StatusPill status={draft.status || 'draft'} />
            <span style={{ color: 'var(--muted)', fontSize: 12 }}>{draft.date} · {draft.readMinutes || '5 мин'}</span>
          </div>
          <h1 style={{ fontFamily: 'var(--font-unbounded)', letterSpacing: 0, lineHeight: 1.1 }}>{draft.title || 'Без названия'}</h1>
          {blogPostCoverUrl(draft) && (
            // eslint-disable-next-line @next/next/no-img-element
            <img src={blogPostCoverUrl(draft)} alt="" style={{ width: '100%', maxHeight: 360, objectFit: 'cover', borderRadius: 8, border: '1px solid var(--line)' }} />
          )}
          <div style={{ marginTop: 20 }}><EditableLegalBody body={draft.body || draft.excerpt || ''} /></div>
        </article>
      )}

      {!loading && tab === 'settings' && (
        <section style={{ maxWidth: 820 }}>
          <BooleanField ckey="features.blog_enabled" label="Показывать блог" hint="Выключение убирает блог из навигации и закрывает прямые маршруты без релиза." items={items} onSave={saveContent} />
          <StringField ckey="landing.blog_button_label" label="Текст кнопки блога" placeholder="Блог" items={items} onSave={saveContent} />
          <StringField ckey="blog.hero.title" label="Заголовок блога" placeholder="Блог Mindstrata" items={items} onSave={saveContent} />
          <StringField ckey="blog.hero.subtitle" label="Подзаголовок блога" placeholder="Короткие разборы методик, выступлений и сценариев применения ИИ." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.closed.title" label="Заголовок закрытого блога" placeholder="Блог пока закрыт" items={items} onSave={saveContent} />
          <StringField ckey="blog.closed.text" label="Текст закрытого блога" placeholder="Раздел скоро откроется." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.empty_state" label="Пустое состояние" placeholder="В этой теме пока нет опубликованных статей." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.subscribe.title" label="Заголовок CTA под блогом" placeholder="Раз в две недели — один полезный разбор" items={items} onSave={saveContent} />
          <StringField ckey="blog.subscribe.text" label="Текст CTA под блогом" placeholder="Без новостной ленты. Только методика, сценарий или практический вывод." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.subscribe.button_label" label="Кнопка CTA под блогом" placeholder="Войти и попробовать" items={items} onSave={saveContent} />
          <StringField ckey="blog.article.back_label" label="Ссылка назад в статье" placeholder="Все статьи" items={items} onSave={saveContent} />
          <StringField ckey="blog.article.not_found_title" label="Заголовок, если статья не найдена" placeholder="Статья не найдена" items={items} onSave={saveContent} />
          <StringField ckey="blog.article.not_found_text" label="Текст, если статья не найдена" placeholder="Она могла быть снята с публикации или ещё не опубликована." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.article.cta_text" label="Текст CTA в статье" placeholder="Можно сразу попробовать режимы на своём вопросе." items={items} onSave={saveContent} multiline />
          <StringField ckey="blog.article.chat_label" label="Кнопка чата в статье" placeholder="Открыть чат" items={items} onSave={saveContent} />
          <StringField ckey="blog.article.login_label" label="Кнопка входа в статье" placeholder="Войти" items={items} onSave={saveContent} />
        </section>
      )}
    </div>
  )
}

function normalizePost(post: BlogPostContent): BlogPostContent {
  return {
    ...emptyPost(),
    ...post,
    slug: slugify(post.slug || post.title || ''),
    date: String(post.date || new Date().toISOString().slice(0, 10)).slice(0, 10),
    status: post.status || 'draft',
    updatedAt: post.updatedAt || new Date().toISOString(),
  }
}

function validateSlug(slug: string, posts: BlogPostContent[], selectedSlug: string) {
  const errors: string[] = []
  if (!slug.trim()) errors.push('Slug обязателен')
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) errors.push('Slug: только латиница, цифры и дефис между словами')
  if (posts.some((post) => post.slug === slug && post.slug !== selectedSlug)) errors.push('Такой slug уже есть')
  return errors
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9а-яё]+/gi, '-')
    .replace(/[а-яё]/gi, '')
    .replace(/^-+|-+$/g, '')
    .replace(/-{2,}/g, '-')
}

function extractSiteMediaUrls(markdown: string): string[] {
  const urls = new Set<string>()
  const re = /!\[[^\]]*]\((\/api\/public\/site-media\/\d+\/[^)\s]+)\)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(markdown)) !== null) {
    urls.add(match[1])
  }
  return Array.from(urls)
}

function siteMediaIdFromUrl(url: string): number | null {
  const match = url.match(/\/api\/public\/site-media\/(\d+)\//)
  if (!match) return null
  const id = Number(match[1])
  return Number.isFinite(id) && id > 0 ? id : null
}

function removeMediaMarkdown(markdown: string, url: string): string {
  const escaped = url.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return markdown
    .replace(new RegExp(`\\n*!?\\[[^\\]]*]\\(${escaped}\\)\\n*`, 'g'), '\n\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

function formatDateTime(value?: string) {
  if (!value) return 'не обновлялась'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 12 }}>
      <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--muted)' }}>{label}</span>
      {children}
      {hint && <span style={{ fontSize: 11, color: 'var(--muted)' }}>{hint}</span>}
    </label>
  )
}

function StatusPill({ status }: { status: BlogPostStatus }) {
  const label = status === 'published' ? 'опубликована' : status === 'scheduled' ? 'запланирована' : 'черновик'
  return <span style={{ padding: '4px 8px', borderRadius: 999, background: 'var(--soft)', color: status === 'published' ? 'var(--accent-strong)' : 'var(--muted)', border: '1px solid var(--line)', fontSize: 11, fontWeight: 600 }}>{label}</span>
}

function CoverDropzone({ cover, onPick, onDrop }: { cover: string; onPick: () => void; onDrop: (file: File) => void }) {
  const [over, setOver] = useState(false)
  return (
    <button
      type="button"
      onClick={onPick}
      onDragOver={(e) => { e.preventDefault(); setOver(true) }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setOver(false)
        const file = e.dataTransfer.files?.[0]
        if (file) onDrop(file)
      }}
      style={{
        width: '100%',
        minHeight: 150,
        borderRadius: 8,
        border: `1.5px dashed ${over ? 'var(--accent)' : 'var(--line)'}`,
        background: over ? 'color-mix(in oklab, var(--accent) 10%, var(--card))' : 'var(--soft)',
        color: 'var(--muted)',
        cursor: 'pointer',
        overflow: 'hidden',
        fontFamily: 'inherit',
      }}
    >
      {cover ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={cover} alt="" style={{ width: '100%', height: 150, objectFit: 'cover', display: 'block' }} />
      ) : (
        <span>Перетащите обложку или нажмите для выбора</span>
      )}
    </button>
  )
}

function Toolbar({ onInsert, onImage }: { onInsert: (value: string) => void; onImage: () => void }) {
  const buttons = [
    ['H2', '\n\n## '],
    ['H3', '\n\n### '],
    ['B', '**текст**'],
    ['I', '*текст*'],
    ['Список', '\n- '],
    ['Цитата', '\n\n> '],
    ['Ссылка', '[текст](https://)'],
    ['Код', '\n```\n\n```\n'],
  ]
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 5, marginBottom: 6 }}>
      {buttons.map(([label, value]) => (
        <button key={label} type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => onInsert(value)}>{label}</button>
      ))}
      <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={onImage}>Картинка</button>
    </div>
  )
}

const hintStyle = { margin: '4px 0 0', color: 'var(--muted)', fontSize: 13, lineHeight: 1.45 }
const tabsStyle = { display: 'flex', gap: 6, marginBottom: 16, borderBottom: '1px solid var(--line)', overflowX: 'auto' } as const
const tabStyle = { padding: '8px 14px', border: 'none', borderBottom: '2px solid transparent', background: 'transparent', color: 'var(--muted)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 13 } as const
const activeTabStyle = { ...tabStyle, color: 'var(--foreground)', borderBottom: '2px solid var(--accent)', fontWeight: 600 } as const
const inputStyle = { width: '100%', padding: '8px 12px', borderRadius: 8, border: '1px solid var(--line)', background: 'var(--card)', color: 'var(--foreground)', fontFamily: 'inherit', fontSize: 14 } as const
const textareaStyle = { ...inputStyle, resize: 'vertical', lineHeight: 1.5 } as const
const sideCardStyle = { padding: 14, border: '1px solid var(--line)', borderRadius: 8, background: 'var(--card)' } as const
const thStyle = { padding: '9px 10px', borderBottom: '1px solid var(--line)', fontWeight: 600 } as const
const tdStyle = { padding: '10px', verticalAlign: 'top' } as const
const linkButtonStyle = { border: 0, padding: 0, background: 'transparent', color: 'var(--foreground)', font: 'inherit', fontWeight: 600, cursor: 'pointer', textAlign: 'left' as const }
const checkStyle = { display: 'flex', gap: 8, alignItems: 'center', fontSize: 13, color: 'var(--foreground)', marginTop: 8 } as const
