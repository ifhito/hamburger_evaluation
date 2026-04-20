module Reviews
  class ReviewFinder
    def initialize(params)
      @params = params
    end

    def search
      scope = Review.kept
      scope = scope.by_rating(@params[:rating])       if @params[:rating].present?
      scope = scope.keyword_search(@params[:keyword]) if @params[:keyword].present?
      if @params[:shop_id].present?
        scope = scope.joins(burger: :shops_and_burgers)
                     .where(shops_and_burgers: { shop_id: @params[:shop_id] })
      end
      scope.recent.includes(:user, :burger, burger: :burger_stat)
    end
  end
end
