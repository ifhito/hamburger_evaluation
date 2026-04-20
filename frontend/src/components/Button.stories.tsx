import type { Meta, StoryObj } from '@storybook/react'
import { Button } from './Button'

const meta: Meta<typeof Button> = {
  component: Button,
  title: 'Components/Button',
  tags: ['autodocs'],
}
export default meta
type Story = StoryObj<typeof Button>

export const Primary: Story = { args: { children: 'Submit', variant: 'primary' } }
export const Secondary: Story = { args: { children: 'Cancel', variant: 'secondary' } }
export const Danger: Story = { args: { children: 'Delete', variant: 'danger' } }
export const Loading: Story = { args: { children: 'Submit', isLoading: true } }
export const Disabled: Story = { args: { children: 'Disabled', disabled: true } }
