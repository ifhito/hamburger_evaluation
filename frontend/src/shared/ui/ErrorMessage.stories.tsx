import type { Meta, StoryObj } from '@storybook/react'
import { ErrorMessage } from './ErrorMessage'

const meta: Meta<typeof ErrorMessage> = {
  component: ErrorMessage,
  title: 'Shared/UI/ErrorMessage',
  tags: ['autodocs'],
}
export default meta
type Story = StoryObj<typeof ErrorMessage>

export const Single: Story = { args: { message: 'Something went wrong.' } }
export const Multiple: Story = { args: { message: ['Email is invalid', 'Password is too short'] } }
