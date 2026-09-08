import { useEffect, useRef } from 'react'
import { useAppStore } from '../stores/app-store'
import { useAuthStore } from '../stores/auth-store'
import { useFamilyStore } from '../stores/family-store'
import { loadHandwrittenFont } from '../utils/font'

export function useBootstrap() {
  const retryKey = useAppStore(state => state.retryKey)
  const running = useRef(false)

  useEffect(() => {
    // 自定义字体预加载，不阻塞 bootstrap
    void loadHandwrittenFont()

    async function bootstrap() {
      if (running.current) {
        return
      }

      running.current = true

      const {
        setBootstrapCompleted,
        setBootstrapError,
        setGlobalLoading,
      } = useAppStore.getState()
      const {
        clearSession,
        hydrate,
      } = useAuthStore.getState()
      const { clearFamily } = useFamilyStore.getState()

      setBootstrapCompleted(false)
      setBootstrapError('')
      setGlobalLoading(true)

      try {
        await hydrate()

        setBootstrapCompleted(true)
      }
      catch {
        await clearSession()
        clearFamily()
        setBootstrapError('启动失败，请检查网络后重试')
        setBootstrapCompleted(true)
      }
      finally {
        setGlobalLoading(false)
        running.current = false
      }
    }

    void bootstrap()
  }, [retryKey])
}
