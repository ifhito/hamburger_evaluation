import type { Meta, StoryObj } from '@storybook/react'
import { Textarea } from './Textarea'

const meta: Meta<typeof Textarea> = {
  component: Textarea,
  title: 'Components/Textarea',
  tags: ['autodocs'],
}
export default meta
type Story = StoryObj<typeof Textarea>

export const Default: Story = { args: { id: 'comment', label: 'Comment', placeholder: 'Write your review…' } }
export const WithError: Story = { args: { id: 'comment', label: 'Comment', error: 'Comment is required' } }
