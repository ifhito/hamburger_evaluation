class Review < ApplicationRecord
  include Discard::Model

  belongs_to :user
  belongs_to :burger

  validates :rating, presence: true, numericality: { only_integer: true, in: 1..5 }
  validates :comment, presence: true

  scope :recent,          -> { order(created_at: :desc) }
  scope :by_rating,       ->(rating) { where(rating: rating) }
  scope :keyword_search,  ->(kw)     { where("comment ILIKE ?", "%#{sanitize_sql_like(kw)}%") }

  after_create_commit { Reviews::ReviewEvents.review_changed_for_burger(burger_id) }
  after_update_commit { Reviews::ReviewEvents.review_changed_for_burger(burger_id) if saved_change_to_rating? }
  after_discard       { Reviews::ReviewEvents.review_changed_for_burger(burger_id) }
end
