module Shops
  class AdminShopQuery
    def search(status: nil)
      scope = Shop.all
      scope = scope.where(status: status) if status.present?
      scope.order(created_at: :desc).includes(:creator)
    end
  end
end
