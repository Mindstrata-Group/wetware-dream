import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useAutoSave } from './useAutoSave'

describe('useAutoSave', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('idle если value === originalValue', () => {
    const onSave = vi.fn()
    const { result } = renderHook(() =>
      useAutoSave({ ckey: 'k', value: 'foo', originalValue: 'foo', onSave })
    )
    expect(result.current.kind).toBe('idle')
    expect(onSave).not.toHaveBeenCalled()
  })

  it('переходит pending → saving → saved при изменении', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) =>
        useAutoSave({ ckey: 'k', value, originalValue: 'orig', onSave, delayMs: 100 }),
      { initialProps: { value: 'orig' } }
    )
    expect(result.current.kind).toBe('idle')

    rerender({ value: 'new' })
    expect(result.current.kind).toBe('pending')
    expect(onSave).not.toHaveBeenCalled()

    // Run the timer.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(100)
    })
    expect(onSave).toHaveBeenCalledWith('k', 'new')
    expect(result.current.kind).toBe('saved')

    // Returns to idle after 2500ms.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2600)
    })
    expect(result.current.kind).toBe('idle')
  })

  it('error если save бросает', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('Boom'))
    const { result, rerender } = renderHook(
      ({ value }: { value: string }) =>
        useAutoSave({ ckey: 'k', value, originalValue: 'a', onSave, delayMs: 50 }),
      { initialProps: { value: 'a' } }
    )

    rerender({ value: 'b' })
    // advance timer → triggers debounce → saving → rejected promise
    await act(async () => {
      await vi.advanceTimersByTimeAsync(50)
      // a few more micro-ticks so the catch runs
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(result.current).toEqual({ kind: 'error', msg: 'Boom' })
  })

  it('последовательные правки дебаунсятся (1 POST в итоге)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const { rerender } = renderHook(
      ({ value }: { value: string }) =>
        useAutoSave({ ckey: 'k', value, originalValue: 'a', onSave, delayMs: 100 }),
      { initialProps: { value: 'a' } }
    )

    rerender({ value: 'b' })
    await act(async () => { await vi.advanceTimersByTimeAsync(50) })
    rerender({ value: 'bc' })
    await act(async () => { await vi.advanceTimersByTimeAsync(50) })
    rerender({ value: 'bcd' })
    await act(async () => { await vi.advanceTimersByTimeAsync(100) })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith('k', 'bcd')
  })

  it('работает с массивами (глубокое сравнение через JSON)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const { rerender } = renderHook(
      ({ value }: { value: number[] }) =>
        useAutoSave({ ckey: 'k', value, originalValue: [1, 2], onSave, delayMs: 50 }),
      { initialProps: { value: [1, 2] } }
    )
    expect(onSave).not.toHaveBeenCalled()

    rerender({ value: [1, 2, 3] })
    await act(async () => { await vi.advanceTimersByTimeAsync(50) })
    expect(onSave).toHaveBeenCalledWith('k', [1, 2, 3])
  })

  it('не сохраняет если value вернулся к original до истечения дебаунса', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const { rerender } = renderHook(
      ({ value }: { value: string }) =>
        useAutoSave({ ckey: 'k', value, originalValue: 'orig', onSave, delayMs: 100 }),
      { initialProps: { value: 'orig' } }
    )

    rerender({ value: 'temp' })   // changed
    await act(async () => { await vi.advanceTimersByTimeAsync(50) })
    rerender({ value: 'orig' })   // reverted
    await act(async () => { await vi.advanceTimersByTimeAsync(100) })

    expect(onSave).not.toHaveBeenCalled()
  })
})
