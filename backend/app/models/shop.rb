class Shop < ApplicationRecord
  has_many :burgers, dependent: :destroy
  has_one  :shop_stat

  validates :name, presence: true
end
