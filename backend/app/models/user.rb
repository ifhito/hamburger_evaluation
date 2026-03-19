class User < ApplicationRecord
  include Discard::Model

  has_secure_password
  has_many :reviews, dependent: :destroy

  validates :email, presence: true, uniqueness: true
  validates :username, presence: true
end
