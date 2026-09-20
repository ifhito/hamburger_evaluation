class Shop < ApplicationRecord
  # 永続化用の状態マッピング。判定/遷移ロジックはドメイン(Shops::ShopEntity / ShopStatus)に集約。
  enum :status, { pending: 0, active: 1, rejected: 2 }

  belongs_to :creator, class_name: "User", optional: true

  has_many :shops_and_burgers, dependent: :destroy
  has_many :burgers, through: :shops_and_burgers

  validates :name, presence: true
end
