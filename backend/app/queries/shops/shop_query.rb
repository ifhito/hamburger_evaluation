module Shops
  class ShopQuery
    def search(keyword: nil)
      scope = Shop.includes(:shop_stat)
      scope = scope.where("name ILIKE ?", "%#{Shop.sanitize_sql_like(keyword)}%") if keyword.present?
      scope.order(:name)
    end

    def find!(id)
      Shop.includes(:shop_stat).find(id)
    end
  end
end
