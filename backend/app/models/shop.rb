class Shop < ApplicationRecord
  has_many :shops_and_burgers, dependent: :destroy
  has_many :burgers, through: :shops_and_burgers
end
