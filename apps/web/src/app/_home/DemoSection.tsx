'use client'

import type { Conversation } from './types'
import { FALLBACK_CONVERSATIONS } from './constants'
import { DemoCard } from './DemoCard'

type DemoSectionProps = {
  activeConversation?: Conversation;
  msgIndex: number;
  typedChars: number;
  dirigentShown: boolean;
  isDesktop: boolean;
  ctaHref: string;
  ctaLabel: string;
}

export function DemoSection({
  activeConversation,
  msgIndex,
  typedChars,
  dirigentShown,
  isDesktop,
  ctaHref,
  ctaLabel,
}: DemoSectionProps) {
  return (
    <div style={{
      flex: 1, minWidth: 0, minHeight: 0,
      height: isDesktop ? 'auto' : '100%',
    }}>
      <DemoCard
        conversation={activeConversation ?? FALLBACK_CONVERSATIONS[0]}
        msgIndex={msgIndex}
        typedChars={typedChars}
        dirigentShown={dirigentShown}
        height={isDesktop ? 420 : '100%'}
        ctaHref={ctaHref}
        ctaLabel={ctaLabel}
      />
    </div>
  )
}
