import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import {
  ActiveFiltersBar,
  CopyIconButton,
  SortHeader,
  TableFrame,
  applySort,
  buildQuery,
  compareDate,
  compareNumber,
  compareText,
  copyText,
  formatRelativeTime,
  formatRequester,
  formatTimeWithTitle,
  shortID,
  type SortState,
} from './ui'

describe('ui helpers', () => {
  it('formats ids, requesters, and query strings predictably', () => {
    expect(shortID('abcdef123456', 6)).toBe('abcdef…')
    expect(formatRequester('user-1', 'Ada', 'ada@example.com', 'agent-7')).toContain('Requested by Ada (ada@example.com) via agent-7')
    expect(buildQuery({ tenant_id: 'tenant-1', decision: '', page: 2 })).toBe('?tenant_id=tenant-1&page=2')
  })

  it('encodes query parameters and requester fallbacks for edge values', () => {
    expect(buildQuery({ tool: 'slack bot', action: 'msg/post', empty: '   ' })).toBe('?tool=slack+bot&action=msg%2Fpost')
    expect(formatRequester('', '', '', '')).toBe('Requested without user or agent attribution')
    expect(formatRequester('', '', '', 'agent-9')).toBe('Requested via agent-9')
  })

  it('keeps zero-like query values while dropping nullish and blank ones', () => {
    expect(buildQuery({ page: 0, risk_min: 0, tenant_id: ' tenant-1 ', empty: '', missing: undefined, none: null })).toBe('?page=0&risk_min=0&tenant_id=tenant-1')
  })

  it('compares text, numbers, dates, and applies sort direction', () => {
    expect(compareText('beta', 'Alpha')).toBeGreaterThan(0)
    expect(compareNumber(9, 3)).toBe(6)
    expect(compareDate('2026-03-23T12:00:00Z', '2026-03-23T11:00:00Z')).toBe(3600000)
    expect(applySort(5, 'desc')).toBe(-5)
  })

  it('shows relative time labels with exact titles', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-23T12:00:00Z'))
    expect(formatRelativeTime('2026-03-23T11:30:00Z')).toMatch(/30 minutes ago|31 minutes ago|29 minutes ago/)
    const time = formatTimeWithTitle('2026-03-23T11:30:00Z')
    expect(time.label).toBeTruthy()
    expect(time.title).toBeTruthy()
  })

  it('toggles sort headers by click and keyboard', async () => {
    const user = userEvent.setup()
    const handleSort = vi.fn()
    const sortState: SortState<'risk'> = { key: null, dir: 'desc' }

    render(
      <SortHeader
        label="Risk"
        sortKey="risk"
        sortState={sortState}
        onSortChange={handleSort}
        defaultDir="desc"
      />,
    )

    const button = screen.getByRole('button', { name: /risk/i })
    await user.click(button)
    expect(handleSort).toHaveBeenLastCalledWith('risk', 'desc')

    fireEvent.keyDown(button, { key: 'Enter' })
    expect(handleSort).toHaveBeenLastCalledWith('risk', 'desc')
  })

  it('renders active filter chips and clear-all actions', async () => {
    const user = userEvent.setup()
    const removeOne = vi.fn()
    const clearAll = vi.fn()

    render(
      <ActiveFiltersBar
        resultCount={3}
        chips={[
          { key: 'tenant', label: 'tenant: demo', onRemove: removeOne },
          { key: 'decision', label: 'decision: approve', onRemove: vi.fn() },
        ]}
        onClearAll={clearAll}
      />,
    )

    expect(screen.getByText(/showing 3 results/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /tenant: demo/i }))
    expect(removeOne).toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: /clear all/i }))
    expect(clearAll).toHaveBeenCalled()
  })

  it('shows a scroll hint when a table frame overflows', () => {
    render(
      <TableFrame>
        <table>
          <tbody>
            <tr>
              <td>Cell</td>
            </tr>
          </tbody>
        </table>
      </TableFrame>,
    )

    const frame = screen.getByRole('table').parentElement as HTMLDivElement
    Object.defineProperty(frame, 'scrollWidth', { configurable: true, value: 600 })
    Object.defineProperty(frame, 'clientWidth', { configurable: true, value: 200 })
    Object.defineProperty(frame, 'scrollLeft', { configurable: true, value: 0 })
    fireEvent(window, new Event('resize'))

    expect(screen.getByText(/scroll/i)).toBeInTheDocument()
  })

  it('copies text through the clipboard helper and button', async () => {
    const user = userEvent.setup()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })

    await copyText('session-123')
    expect(writeText).toHaveBeenCalledWith('session-123')

    render(<CopyIconButton text="session-123" label="Session ID" />)
    await user.click(screen.getByRole('button', { name: /copy session id/i }))
    expect(writeText).toHaveBeenCalledWith('session-123')
  })

  it('falls back to document.execCommand when the async clipboard API is unavailable', async () => {
    const originalClipboard = navigator.clipboard
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    })
    const execCommandSpy = vi.spyOn(document, 'execCommand').mockReturnValue(true)

    await copyText('fallback-copy')

    expect(execCommandSpy).toHaveBeenCalledWith('copy')
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: originalClipboard,
    })
  })
})
