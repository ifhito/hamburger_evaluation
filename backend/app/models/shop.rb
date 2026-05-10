class Shop < ApplicationRecord
  has_many :burgers, dependent: :destroy

  validates :name, presence: true
end
