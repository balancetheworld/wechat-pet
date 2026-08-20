import type { PropsWithChildren } from 'react'
import { useBootstrap } from './hooks/use-bootstrap'

import './app.scss'

function App({ children }: PropsWithChildren<any>) {
  useBootstrap()

  // children 是将要会渲染的页面
  return children
}

export default App
