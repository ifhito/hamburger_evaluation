module Shops
  class ShopQuery
    def search(keyword: nil)
      scope = Shop.all
      scope = scope.where("name ILIKE ?", "%#{Shop.sanitize_sql_like(keyword)}%") if keyword.present?
      scope.order(:name)
    end

    def find!(id)
      Shop.find(id)
    end
  end
end
