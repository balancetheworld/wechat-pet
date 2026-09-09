import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useBootstrap } from './hooks/use-bootstrap'
import { queryClient } from './services/query-client'

import './app.scss'

function App({ children }: PropsWithChildren) {
  useBootstrap()

  // children 是将要会渲染的页面
  return createElement(QueryClientProvider, { client: queryClient }, children)
}

export default App
