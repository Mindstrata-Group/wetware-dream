import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TopBar } from './TopBar'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}))

const featureLinks = [
  { href: '/blog', label: 'Блог' },
  { href: '/shop', label: 'Магазин решений' },
]

describe('home feature links', () => {
  afterEach(() => cleanup())

  it('shows blog and shop buttons in the top navigation when feature flags are enabled', () => {
    render(
      <TopBar
        topbarH={68}
        padH={32}
        isFold={false}
        isMobile={false}
        entryHref="/login"
        entryLabel="Войти"
        profileRole={null}
        featureLinks={featureLinks}
        onLogout={() => {}}
      />,
    )

    expect(screen.getByRole('link', { name: 'Блог' }).getAttribute('href')).toBe('/blog')
    expect(screen.getByRole('link', { name: 'Магазин решений' }).getAttribute('href')).toBe('/shop')
    expect(screen.getByRole('link', { name: 'Войти' }).getAttribute('href')).toBe('/login')
  })
})
