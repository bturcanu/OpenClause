import { type ReactElement } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { render } from '@testing-library/react'

type RenderRouteOptions = {
  path?: string
  route?: string
}

export function renderRoute(element: ReactElement, options: RenderRouteOptions = {}) {
  const { path = '/', route = path } = options
  return render(
    <MemoryRouter initialEntries={[route]}>
      <Routes>
        <Route path={path} element={element} />
      </Routes>
    </MemoryRouter>,
  )
}
