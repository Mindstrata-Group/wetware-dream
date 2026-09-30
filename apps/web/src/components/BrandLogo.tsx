import Link from 'next/link'

type BrandLogoProps = {
  href?: string
  variant?: 'horizontal' | 'vertical' | 'mark'
  width?: number
  height?: number
  priority?: boolean
  className?: string
}

const variants = {
  horizontal: {
    src: '/brand/logo-horizontal.png',
    width: 286,
    height: 92,
    alt: 'СТРАТУМ Mindstrata',
  },
  vertical: {
    src: '/brand/logo-vertical.png',
    width: 132,
    height: 148,
    alt: 'СТРАТУМ Mindstrata',
  },
  mark: {
    src: '/brand/logo-mark.png',
    width: 94,
    height: 94,
    alt: 'СТРАТУМ Mindstrata',
  },
} as const

export function BrandLogo({
  href = '/',
  variant = 'horizontal',
  width,
  height,
  priority = false,
  className,
}: BrandLogoProps) {
  const asset = variants[variant]

  return (
    <Link href={href} className={className} aria-label="СТРАТУМ Mindstrata">
      <img
        src={asset.src}
        alt={asset.alt}
        width={width ?? asset.width}
        height={height ?? asset.height}
        loading={priority ? 'eager' : 'lazy'}
        decoding="async"
        fetchPriority={priority ? 'high' : 'auto'}
        className="ms-logo-image"
      />
    </Link>
  )
}
