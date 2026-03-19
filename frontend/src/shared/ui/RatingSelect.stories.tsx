import type { Meta, StoryObj } from '@storybook/react'
import { useState } from 'react'
import { RatingSelect } from './RatingSelect'

const meta: Meta<typeof RatingSelect> = {
  component: RatingSelect,
  title: 'Shared/UI/RatingSelect',
  tags: ['autodocs'],
}
export default meta
type Story = StoryObj<typeof RatingSelect>

function Controlled() {
  const [value, setValue] = useState(3)
  return <RatingSelect value={value} onChange={setValue} />
}

export const Default: Story = { render: () => <Controlled /> }
export const WithError: Story = {
  render: () => <RatingSelect value={0} onChange={() => {}} error="Rating is required" />,
}
