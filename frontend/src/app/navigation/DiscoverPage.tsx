import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Layout } from './Layout'
import { ShopListContent } from '../../domains/shops/pages/ShopListPage'
import { BurgerListContent } from '../../domains/burgers/pages/BurgerListPage'
import { ReviewListContent } from '../../domains/reviews/pages/ReviewListPage'
import styles from './navigation.module.css'

export default function DiscoverPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const target = params.get('type')
  const selected = target === 'burgers' || target === 'reviews' ? target : 'shops'
  return <Layout title={t('navigation.discover')}>
    <p className={styles.lead}>{t('navigation.discoverLead')}</p>
    <nav className={styles.tabs} aria-label={t('navigation.targets')}>
      {(['shops', 'burgers', 'reviews'] as const).map(type => <Link key={type} to={`/discover?type=${type}`} aria-current={selected === type ? 'page' : undefined}>{t(`navigation.${type}`)}</Link>)}
    </nav>
    {selected === 'shops' ? <ShopListContent hideHeading /> : selected === 'burgers' ? <BurgerListContent hideHeading /> : <ReviewListContent hideHeading />}
  </Layout>
}
