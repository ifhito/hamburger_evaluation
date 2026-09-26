// @vitest-environment jsdom
import { act } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import '../../lib/i18n'
import { cleanup, click, mount, need } from '../../test/dom'
import DiscoverPage from './DiscoverPage'
vi.mock('../../domains/auth/AuthProvider', () => ({ useAuth: () => ({ user: null }) }))
vi.mock('../../domains/shops/pages/ShopListPage', () => ({ ShopListContent: () => <p>Shop results</p> }))
vi.mock('../../domains/burgers/pages/BurgerListPage', () => ({ BurgerListContent: () => <p>Burger results</p> }))
vi.mock('../../domains/reviews/pages/ReviewListPage', () => ({ ReviewListContent: () => <p>Review results</p> }))
afterEach(cleanup)
it('検索対象を切り替えると対象の一覧だけを表示し、戻る操作でも対象を復元する', async () => {
  const router = createMemoryRouter([{ path: '/discover', element: <DiscoverPage /> }], { initialEntries: ['/discover'] })
  const page = await mount(<RouterProvider router={router} />)
  expect(page.textContent).toContain('Shop results')
  await click(need(page.querySelector('a[href="/discover?type=burgers"]'), 'バーガー'))
  expect(page.textContent).toContain('Burger results')
  expect(page.textContent).not.toContain('Shop results')
  await click(need(page.querySelector('a[href="/discover?type=reviews"]'), 'レビュー'))
  expect(page.textContent).toContain('Review results')
  await act(() => router.navigate(-1))
  expect(page.textContent).toContain('Burger results')
})
