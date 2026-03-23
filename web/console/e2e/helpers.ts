import { expect, type Page } from '@playwright/test'

const ADMIN_EMAIL = process.env.CONSOLE_ADMIN_EMAIL || 'admin@openclause.dev'
const ADMIN_PASSWORD = process.env.CONSOLE_ADMIN_PASSWORD || 'Admin123!'

export async function login(page: Page) {
  await page.goto('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await page.getByLabel('Email').fill(ADMIN_EMAIL)
  await page.getByLabel('Password').fill(ADMIN_PASSWORD)
  await page.getByRole('button', { name: /sign in/i }).click()
  await expect(page.getByRole('heading', { name: /overview/i })).toBeVisible()
}
