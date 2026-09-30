import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EditableLegalBody } from './EditableLegalBody'

describe('EditableLegalBody', () => {
  it('рендерит заголовки h1-h4', () => {
    const md = `# Главный\n\n## Второй\n\n### Третий\n\n#### Четвёртый`
    render(<EditableLegalBody body={md} />)
    expect(screen.getByRole('heading', { level: 1, name: 'Главный' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 2, name: 'Второй' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 3, name: 'Третий' })).toBeTruthy()
    expect(screen.getByRole('heading', { level: 4, name: 'Четвёртый' })).toBeTruthy()
  })

  it('рендерит параграфы', () => {
    render(<EditableLegalBody body={'Первый абзац.\n\nВторой абзац.'} />)
    expect(screen.getByText('Первый абзац.')).toBeTruthy()
    expect(screen.getByText('Второй абзац.')).toBeTruthy()
  })

  it('рендерит маркированный список', () => {
    const md = '- один\n- два\n- три'
    render(<EditableLegalBody body={md} />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0].textContent).toBe('один')
    expect(items[2].textContent).toBe('три')
  })

  it('рендерит нумерованный список', () => {
    const md = '1. первый\n2. второй'
    render(<EditableLegalBody body={md} />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0].textContent).toBe('первый')
  })

  it('рендерит жирный и курсив', () => {
    render(<EditableLegalBody body={'**жирный** и *курсив*'} />)
    expect(screen.getByText('жирный').tagName).toBe('STRONG')
    expect(screen.getByText('курсив').tagName).toBe('EM')
  })

  it('рендерит зачёркнутый (GFM)', () => {
    render(<EditableLegalBody body={'~~старое~~'} />)
    expect(screen.getByText('старое').tagName).toBe('DEL')
  })

  it('внутренние ссылки идут через Next Link', () => {
    render(<EditableLegalBody body={'[документ](/privacy/terms)'} />)
    const link = screen.getByRole('link', { name: 'документ' })
    expect(link.getAttribute('href')).toBe('/privacy/terms')
    expect(link.getAttribute('target')).toBeNull()
  })

  it('внешние ссылки открываются в новой вкладке', () => {
    render(<EditableLegalBody body={'[google](https://google.com)'} />)
    const link = screen.getByRole('link', { name: 'google' })
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toContain('noopener')
  })

  it('рендерит таблицы (GFM)', () => {
    const md = '| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |'
    render(<EditableLegalBody body={md} />)
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.getByRole('columnheader', { name: 'A' })).toBeTruthy()
    expect(screen.getByRole('cell', { name: '3' })).toBeTruthy()
  })

  it('рендерит blockquote', () => {
    const { container } = render(<EditableLegalBody body={'> цитата'} />)
    expect(container.querySelector('blockquote')).toBeTruthy()
    expect(container.querySelector('blockquote')?.textContent).toContain('цитата')
  })

  it('рендерит inline-код', () => {
    render(<EditableLegalBody body={'переменная `x = 1`'} />)
    expect(screen.getByText('x = 1').tagName).toBe('CODE')
  })

  it('рендерит горизонтальную линию', () => {
    const { container } = render(<EditableLegalBody body={'a\n\n---\n\nb'} />)
    expect(container.querySelector('hr')).toBeTruthy()
  })

  it('пустой body не падает', () => {
    const { container } = render(<EditableLegalBody body={''} />)
    expect(container).toBeTruthy()
  })
})
