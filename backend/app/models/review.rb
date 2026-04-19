class Review < ApplicationRecord
  include Discard::Model

  belongs_to :user
  belongs_to :burger

  validates :rating, presence: true, numericality: { only_integer: true, in: 1..5 }
  validates :comment, presence: true

  scope :recent,          -> { order(created_at: :desc) }
  scope :by_rating,       ->(rating) { where(rating: rating) }
  scope :keyword_search,  ->(kw)     { where("comment ILIKE ?", "%#{sanitize_sql_like(kw)}%") }

  after_create        { BurgerStatUpdateJob.perform_later(burger_id) }
  after_update_commit { BurgerStatUpdateJob.perform_later(burger_id) if saved_change_to_rating? }
  after_discard       { BurgerStatUpdateJob.perform_later(burger_id) }
end
