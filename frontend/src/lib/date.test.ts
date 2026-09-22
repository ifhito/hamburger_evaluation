import { afterEach, describe, expect, it } from 'vitest'
import i18n from './i18n'
import { formatDate } from './date'

describe('formatDate', () => {
  afterEach(() => {
    void i18n.changeLanguage('en')
  })

  it('formats an ISO date string to locale date', () => {
    const result = formatDate('2024-01-15T10:00:00.000Z')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('英語のときは "Sep 21, 2026" の書式(デザインどおり)', async () => {
    await i18n.changeLanguage('en')
    expect(formatDate('2026-09-21T00:00:00.000Z')).toBe('Sep 21, 2026')
  })

  it('日本語に切り替えると "2026年9月21日" の書式になる(R3)', async () => {
    await i18n.changeLanguage('ja')
    expect(formatDate('2026-09-21T00:00:00.000Z')).toBe('2026年9月21日')
  })
})
