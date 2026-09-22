import { useState } from 'react'
import type { Meta, StoryObj } from '@storybook/react'
import '../../lib/i18n'
import { Alert } from './Alert'
import { Badge } from './Badge'
import { Button } from './Button'
import { Card } from './Card'
import { RatingBurger } from './RatingBurger'
import { RatingInput } from './RatingInput'
import { EmptyState, Loading, NotFound } from './states'
import { TextArea, TextField } from './TextField'

// リデザインの部品の一覧(design/redesign/ と見比べる用)。最大・最小などの数字は、GET /meta の値の代わりに渡した例。
const meta: Meta = { title: 'UI/Redesign', parameters: { layout: 'padded' } }
export default meta
type Story = StoryObj

const row = { display: 'flex', flexWrap: 'wrap', gap: 16, alignItems: 'center', marginBottom: 16 } as const

export const Buttons: Story = {
  render: () => (
    <div style={row}>
      <Button>Write a review</Button>
      <Button variant="secondary">Cancel</Button>
      <Button variant="danger">Delete</Button>
      <Button variant="dangerSolid">Delete</Button>
      <Button variant="dark">Confirm reject</Button>
      <Button disabled>Post</Button>
      <Button variant="danger" disabled>
        Delete
      </Button>
      <Button isLoading loadingLabel="Posting…">
        Post
      </Button>
    </div>
  ),
}

export const Badges: Story = {
  render: () => (
    <div style={row}>
      <Badge tone="accent">Public</Badge>
      <Badge>Pending</Badge>
      <Badge>Rejected</Badge>
    </div>
  ),
}

export const Cards: Story = {
  render: () => (
    <div style={{ display: 'grid', gap: 16, maxWidth: 480 }}>
      <Card>A review card</Card>
      <Card current>The review you are looking at</Card>
    </div>
  ),
}

export const Alerts: Story = {
  render: () => (
    <div style={{ display: 'grid', gap: 16, maxWidth: 560 }}>
      <Alert title="Could not save the review" message="Comment is too long (maximum is 2000 characters)" />
      <Alert message={['Name is required', 'Rating is out of range']} />
    </div>
  ),
}

function FieldsDemo() {
  const [text, setText] = useState('')
  return (
    <div style={{ display: 'grid', gap: 16, maxWidth: 480 }}>
      <TextField id="s-name" label="Burger name" hint="Shown on the review" counter={{ value: text, max: 20 }} value={text} onChange={(e) => setText(e.target.value)} />
      <TextArea id="s-comment" label="Comment" optional="(optional)" counter={{ value: text, max: 20 }} value={text} onChange={(e) => setText(e.target.value)} />
      <TextArea id="s-nomax" label="Limit not loaded yet" counter={{ value: text, max: undefined }} />
    </div>
  )
}
export const Fields: Story = { render: () => <FieldsDemo /> }

export const RatingDisplay: Story = {
  render: () => (
    <div style={{ display: 'grid', gap: 24 }}>
      {(['lg', 'md', 'sm', 'xs'] as const).map((size) => (
        <div key={size} style={row}>
          {[0, 1, 2.5, 3.7, 4.5, 5].map((v) => (
            <RatingBurger key={v} value={v} max={5} size={size} />
          ))}
        </div>
      ))}
    </div>
  ),
}

function RatingInputDemo() {
  const [value, setValue] = useState<number | null>(null)
  return (
    <div style={{ display: 'grid', gap: 32 }}>
      <RatingInput label="Rating" value={value} onChange={setValue} min={1} max={5} />
      <RatingInput label="A range from GET /meta" value={value} onChange={setValue} min={1} max={7} />
    </div>
  )
}
export const RatingInputStory: Story = { name: 'RatingInput', render: () => <RatingInputDemo /> }

export const States: Story = {
  render: () => (
    <div style={{ display: 'grid', gap: 24, maxWidth: 560 }}>
      <EmptyState action={<Button>Write a review</Button>} />
      <Loading />
      <NotFound action={<Button variant="secondary">Back to shops</Button>} />
    </div>
  ),
}
