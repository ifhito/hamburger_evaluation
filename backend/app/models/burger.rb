class Burger < ApplicationRecord
  has_many :reviews, dependent: :destroy
  has_many :shops_and_burgers, dependent: :destroy
  has_many :shops, through: :shops_and_burgers
  has_one  :burger_stat

  validates :name, presence: true
end
