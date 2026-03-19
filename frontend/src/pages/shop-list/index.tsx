import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useShops } from '../../shared/lib/hooks/useShops'
import { Input } from '../../shared/ui/Input'
import { Layout } from '../../shared/ui/Layout'

export default function ShopListPage() {
  const [keyword, setKeyword] = useState('')
  const { data: shops, isLoading } = useShops(keyword ? { keyword } : undefined)

  return (
    <Layout title="Shops">
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <Input
          id="keyword"
          label="Search by name"
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder="e.g. Shake Shack"
        />
        {isLoading && <p>Loading…</p>}
        {shops && shops.length === 0 && <p>No shops found.</p>}
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
          {shops?.map((shop) => (
            <li key={shop.id}>
              <Link
                to={`/shops/${shop.id}`}
                style={{
                  display: 'block',
                  padding: '12px 16px',
                  border: '1px solid #ddd',
                  borderRadius: 6,
                  color: 'inherit',
                  textDecoration: 'none',
                  fontWeight: 500,
                }}
              >
                {shop.name}
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </Layout>
  )
}
