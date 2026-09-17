module Shops
  # 一覧で「どの店舗を見せるか」の条件仕様。
  # 規則はドメインが保持し、SQL への翻訳は query 層が担う。
  class VisibilityScope
    attr_reader :statuses, :owner_id

    def self.for(viewer_id: nil, admin: false)
      return new(statuses: Shops::ShopStatus::ALL, owner_id: nil) if admin
      return new(statuses: Shops::ShopStatus::PUBLICLY_LISTED, owner_id: nil) if viewer_id.nil?

      # 一般ユーザー: 公開店舗 + 自分が作成した店舗(全status)
      new(statuses: Shops::ShopStatus::PUBLICLY_LISTED, owner_id: viewer_id)
    end

    def initialize(statuses:, owner_id:)
      @statuses = statuses
      @owner_id = owner_id
      freeze
    end
  end
end
