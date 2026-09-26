import { useTranslation } from 'react-i18next'
import { ShopListContent } from '../../domains/shops/pages/ShopListPage'
import { Layout } from './Layout'
import styles from './navigation.module.css'

export default function RecordPage() {
  const { t } = useTranslation()
  return <Layout title={t('navigation.recordTitle')}>
    <p className={styles.lead}>{t('navigation.recordLead')}</p>
    <ShopListContent hideHeading record />
  </Layout>
}
