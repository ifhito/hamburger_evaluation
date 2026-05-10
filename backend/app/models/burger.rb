class Burger < ApplicationRecord
  belongs_to :shop
  has_many :reviews, dependent: :destroy
  has_one  :burger_stat

  validates :name, presence: true, uniqueness: { scope: :shop_id }
end
