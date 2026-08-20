import { useEffect, useRef } from 'react'
import { login } from '../services/auth'
import { useAppStore } from '../stores/app-store'
import { useAuthStore } from '../stores/auth-store'
import { useFamilyStore } from '../stores/family-store'

export function useBootstrap() {
  const retryKey = useAppStore(state => state.retryKey)
  const running = useRef(false)

  useEffect(() => {
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
        setSession,
      } = useAuthStore.getState()
      const { clearFamily, setFamily } = useFamilyStore.getState()

      setBootstrapCompleted(false)
      setBootstrapError('')
      setGlobalLoading(true)

      try {
        await hydrate()
        const session = await login()

        await setSession(session)

        if (session.family && session.familyRole) {
          setFamily(session.family, session.familyRole)
        }
        else {
          clearFamily()
        }

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
