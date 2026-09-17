class User < ApplicationRecord
  include Discard::Model

  has_secure_password
  has_many :reviews, dependent: :destroy
  has_many :created_shops, class_name: "Shop", foreign_key: :creator_id, dependent: :nullify, inverse_of: :creator

  validates :email, presence: true, uniqueness: true
  validates :username, presence: true
end
