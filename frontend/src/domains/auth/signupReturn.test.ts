// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { consumeSignupReturn, rememberSignupReturn } from './signupReturn'
beforeEach(() => localStorage.clear())
describe('メール登録後の復帰先', () => {
  it('投稿フォームの内部パスを一度だけ返す', () => {
    rememberSignupReturn('/reviews/new?shop_id=9')
    expect(consumeSignupReturn()).toBe('/reviews/new?shop_id=9')
    expect(consumeSignupReturn()).toBeNull()
  })
  it('外部URLと別画面へのパスは保存しない', () => {
    for (const path of ['//example.com', '/\\example.com', '/oauth/authorize?client=other']) {
      rememberSignupReturn(path)
      expect(consumeSignupReturn()).toBeNull()
    }
  })
  it('古い保存値を使用しない', () => {
    localStorage.setItem('burgerstack:signup-return', JSON.stringify({ path: '/record', createdAt: Date.now() - 60 * 60 * 1000 }))
    expect(consumeSignupReturn()).toBeNull()
  })
})
