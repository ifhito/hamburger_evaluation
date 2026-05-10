import type { Meta, StoryObj } from '@storybook/react'
import { Input } from './Input'

const meta: Meta<typeof Input> = {
  component: Input,
  title: 'Components/Input',
  tags: ['autodocs'],
}
export default meta
type Story = StoryObj<typeof Input>

export const Default: Story = { args: { id: 'email', label: 'Email', type: 'email', placeholder: 'you@example.com' } }
export const WithError: Story = { args: { id: 'email', label: 'Email', error: 'Invalid email address', type: 'email' } }
