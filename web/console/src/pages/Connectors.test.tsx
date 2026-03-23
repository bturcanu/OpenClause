import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import Connectors from './Connectors'
import { renderRoute } from '../test/render'
import { mockApiGet } from '../test/mockApi'

describe('Connectors page', () => {
  it('normalizes array responses and supports expanding and collapsing long action lists', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/connectors', [
        {
          tool: 'slack',
          type: 'remote',
          actions: ['msg.post', 'channel.list', 'user.lookup', 'message.pin', 'message.update', 'message.delete', 'conversation.info'],
          event_count: 12,
        },
      ]],
    ])

    renderRoute(<Connectors />, { path: '/connectors', route: '/connectors' })

    expect(await screen.findByText('slack')).toBeInTheDocument()
    expect(screen.getByText('12 events')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /\+1 more/i }))
    expect(screen.getByText('conversation.info')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /show less/i }))
    expect(screen.queryByText('conversation.info')).not.toBeInTheDocument()
  })

  it('shows the empty-search state separately from an empty registry and clears local search', async () => {
    const user = userEvent.setup()
    mockApiGet([
      ['/admin/connectors', {
        connectors: [
          { name: 'slack', type: 'remote', actions: ['msg.post'] },
        ],
      }],
    ])

    renderRoute(<Connectors />, { path: '/connectors', route: '/connectors' })

    expect(await screen.findByText('slack')).toBeInTheDocument()
    await user.type(screen.getByPlaceholderText(/find a connector or action/i), 'jira')
    expect(screen.getByText(/no connectors match this search/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /clear search/i }))
    expect(await screen.findByText('slack')).toBeInTheDocument()
  })
})
