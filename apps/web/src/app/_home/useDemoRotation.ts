'use client'

import { useCallback, useEffect, useState } from 'react'
import type { Conversation } from './types'

export function useDemoRotation(demos: Conversation[]) {
  const [active, setActive] = useState(0)
  const [msgIndex, setMsgIndex] = useState(0)
  const [typedChars, setTypedChars] = useState(0)
  const [dirigentShown, setDirigentShown] = useState(false)

  useEffect(() => {
    let cancelled = false
    const wait = (ms: number) => new Promise<void>(r => setTimeout(r, ms))

    const cycle = async () => {
      while (!cancelled) {
        for (let i = 0; i < demos.length; i++) {
          if (cancelled) return
          setActive(i)
          setMsgIndex(0)
          setTypedChars(0)
          setDirigentShown(false)

          const messages = demos[i]?.messages || []
          let dirigentDone = false

          for (let m = 0; m < messages.length; m++) {
            if (cancelled) return
            setMsgIndex(m)
            setTypedChars(0)

            if (!dirigentDone && messages[m].role === 'assistant') {
              setDirigentShown(true)
              dirigentDone = true
              await wait(1200)
              if (cancelled) return
            }

            const text = messages[m].content
            const charDelay = messages[m].role === 'user' ? 17 : 13
            for (let c = 0; c <= text.length; c++) {
              if (cancelled) return
              setTypedChars(c)
              await wait(charDelay)
            }
            await wait(messages[m].role === 'user' ? 480 : 1000)
          }
          await wait(2200)
        }
      }
    }

    void cycle()
    return () => { cancelled = true }
  }, [demos])

  const pickDemo = useCallback((i: number) => {
    setActive(i)
    const msgs = demos[i]?.messages || []
    setMsgIndex(Math.max(0, msgs.length - 1))
    setTypedChars(msgs[msgs.length - 1]?.content.length ?? 0)
    setDirigentShown(true)
  }, [demos])

  return { active, msgIndex, typedChars, dirigentShown, pickDemo }
}
