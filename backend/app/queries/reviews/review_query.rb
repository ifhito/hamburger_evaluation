module Reviews
  class ReviewQuery
    def initialize(params = {})
      @params = params
    end

    def find_kept!(id)
      Review.kept.includes(:user, :burger, burger: :burger_stat).find(id)
    end

    def search
      scope = Review.kept
      scope = scope.by_rating(@params[:rating])       if @params[:rating].present?
      scope = scope.keyword_search(@params[:keyword]) if @params[:keyword].present?
      if @params[:shop_id].present?
        scope = scope.joins(burger: :shops_and_burgers)
                     .where(shops_and_burgers: { shop_id: @params[:shop_id] })
      else
        # グローバル一覧では公開(active)店舗に紐づく burger のレビューのみ。
        scope = scope.where(burger_id: publicly_listed_burger_ids)
      end
      scope.recent.includes(:user, :burger, burger: :burger_stat)
    end

    private

    def publicly_listed_burger_ids
      ShopsAndBurger
        .where(shop_id: Shop.where(status: Shops::ShopStatus::PUBLICLY_LISTED).select(:id))
        .select(:burger_id)
    end
  end
end
