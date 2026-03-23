import { screen } from '@testing-library/react'

export function getFieldByLabelText(labelText: RegExp | string) {
  const label = screen.getByText(labelText, { selector: 'label' })
  const control = label.parentElement?.querySelector('input, select, textarea')
  if (!control) {
    throw new Error(`No form control found next to label ${String(labelText)}`)
  }
  return control as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
}
