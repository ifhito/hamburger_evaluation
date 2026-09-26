import { useCallback, useEffect, useRef } from 'react'
import { useBeforeUnload, useBlocker } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

export function useUnsavedChanges(dirty: boolean) {
  const { t } = useTranslation()
  const saved = useRef(false)
  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty && !saved.current &&
    currentLocation.pathname + currentLocation.search !== nextLocation.pathname + nextLocation.search)

  useBeforeUnload(useCallback((event) => {
    if (dirty && !saved.current) {
      event.preventDefault()
      event.returnValue = ''
    }
  }, [dirty]))

  useEffect(() => {
    if (blocker.state !== 'blocked') return
    if (window.confirm(t('navigation.unsaved'))) blocker.proceed()
    else blocker.reset()
  }, [blocker, t])

  // 保存成功直後の遷移は、再描画を待たずに通す。失敗時は呼ばない。
  return () => { saved.current = true }
}
