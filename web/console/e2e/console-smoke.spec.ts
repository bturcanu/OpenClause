import { expect, test } from '@playwright/test'
import { login } from './helpers'

test.describe('console browser smoke', () => {
  test('logs in and renders the overview dashboard', async ({ page }) => {
    await login(page)

    await expect(page.getByRole('heading', { name: /overview/i })).toBeVisible()
    await expect(page.getByRole('heading', { name: /event volume/i })).toBeVisible()
    await expect(page.getByText(/total events/i)).toBeVisible()
  })

  test('creates a tenant, registers an agent, and creates an API key', async ({ page }) => {
    const tenantName = `Smoke Tenant ${Date.now()}`
    const agentName = `Smoke Agent ${Date.now()}`
    const keyName = `Smoke Key ${Date.now()}`

    await login(page)
    await page.goto('/tenants')
    await expect(page.getByRole('heading', { name: /tenants/i })).toBeVisible()

    await page.getByRole('button', { name: /\+ new tenant/i }).click()
    await page.getByLabel('Name').fill(tenantName)
    await page.getByRole('button', { name: /^create$/i }).click()

    const tenantRow = page.locator('tr', { hasText: tenantName })
    await expect(tenantRow).toBeVisible()
    await tenantRow.getByRole('link', { name: /open tenant/i }).click()

    await expect(page.getByRole('heading', { name: tenantName })).toBeVisible()
    await page.getByLabel('Agent Name').fill(agentName)
    await page.getByRole('button', { name: /^create$/i }).click()
    await expect(page.getByText(agentName)).toBeVisible()

    await page.getByRole('button', { name: /api keys/i }).click()
    const createKeyCard = page.locator('.form-card', { has: page.getByRole('heading', { name: /create api key/i }) })
    await createKeyCard.getByLabel('Name').fill(keyName)
    await createKeyCard.getByRole('button', { name: /^create$/i }).click()

    await expect(page.getByText(/copy this key now/i)).toBeVisible()
    await expect(page.getByText(keyName)).toBeVisible()
  })

  test('filters audit trail rows and opens an event detail page', async ({ page }) => {
    await login(page)
    await page.goto('/events')

    await expect(page.getByRole('heading', { name: /audit trail/i })).toBeVisible()
    await page.getByLabel('Decision').selectOption('approve')
    await expect(page.getByRole('button', { name: /decision: approve/i })).toBeVisible()

    const openEvent = page.getByRole('link', { name: /open event/i }).first()
    await expect(openEvent).toBeVisible()
    await openEvent.click()

    await expect(page.getByRole('heading', { name: /event detail/i })).toBeVisible()
    await expect(page.getByRole('heading', { name: /hash chain/i })).toBeVisible()
  })

  test('opens the latest run from sessions and shows execution linkage', async ({ page }) => {
    await login(page)
    await page.goto('/sessions')

    await expect(page.getByRole('heading', { name: /sessions/i })).toBeVisible()
    const openRun = page.getByRole('link', { name: /open run/i }).first()
    await expect(openRun).toBeVisible()
    await openRun.click()

    await expect(page.getByRole('heading', { name: /session detail/i })).toBeVisible()
    await expect(page.getByRole('heading', { name: /run context/i })).toBeVisible()
    await expect(page.getByRole('link', { name: /open execution event/i }).first()).toBeVisible()
  })
})
