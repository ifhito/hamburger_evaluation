// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import '../lib/i18n'
import { cleanup, click, mount, need } from '../test/dom'
import { Layout } from './Layout'
const auth = vi.hoisted(() => ({ user: null as null | { id: string } }))
vi.mock('../domains/auth/AuthProvider', () => ({ useAuth: () => ({ user: auth.user, isLoading: false }) }))
const show = (path = '/') => mount(<RouterProvider router={createMemoryRouter([{ path: '*', element: <Layout><h1>Content</h1></Layout> }], { initialEntries: [path] })} />)
beforeEach(() => { auth.user = null })
afterEach(cleanup)
describe('採用ナビゲーション', () => {
  it('ヘッダーに紹介・探す・記録・プロフィールの入口を持つ', async () => {
    const page = await show()
    expect(page.querySelector('header a')?.getAttribute('href')).toBe('/')
    for (const path of ['/about', '/discover', '/record', '/me']) expect(page.querySelector(`header a[href="${path}"]`)).not.toBeNull()
    expect(page.querySelector('footer a[href="/mcp"]')).not.toBeNull()
    expect(page.querySelector('footer button')?.textContent).toBe('日本語')
  })
  it.each(['/discover?type=burgers', '/shops/9', '/burgers/3', '/reviews/7'])('%sで探すを選択する', async path => {
    const page = await show(path)
    expect(page.querySelector('a[href="/discover"][aria-current="page"]')).not.toBeNull()
  })
  it.each(['/record', '/shops/9?from=record', '/reviews/new?shop_id=9', '/reviews/7/edit'])('%sで記録するを選択する', async path => {
    const page = await show(path)
    expect(page.querySelector('a[href="/record"][aria-current="page"]')).not.toBeNull()
    expect(page.querySelector('a[href="/discover"][aria-current]')).toBeNull()
  })
  it('他人のプロフィールではマイページを選択しない', async () => {
    auth.user = { id: '1' }
    const page = await show('/users/2')
    expect(page.querySelector('a[aria-current]')).toBeNull()
  })
  it('本人の設定画面ではマイページを選択する', async () => {
    auth.user = { id: '1' }
    const page = await show('/users/1/edit')
    expect(page.querySelector('a[href="/me"][aria-current="page"]')).not.toBeNull()
  })
  it('紹介ページでは下部ナビを未選択にし、遷移すると本文へフォーカスする', async () => {
    const page = await show('/about')
    const navs = page.querySelectorAll('nav[aria-label="Main navigation"]')
    expect(navs[1]?.querySelector('[aria-current]')).toBeNull()
    await click(need(page.querySelector('header a[href="/discover"]'), '探す'))
    expect(document.activeElement).toBe(page.querySelector('main'))
  })
})
