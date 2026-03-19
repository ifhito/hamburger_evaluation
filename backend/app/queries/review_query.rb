class ReviewQuery
  def self.call(params)
    new(params).call
  end

  def initialize(params)
    @params = params
  end

  def call
    scope = Review.kept
    scope = scope.by_rating(@params[:rating])       if @params[:rating].present?
    scope = scope.keyword_search(@params[:keyword]) if @params[:keyword].present?
    scope = scope.joins(burger: :shops_and_burgers)
                 .where(shops_and_burgers: { shop_id: @params[:shop_id] }) if @params[:shop_id].present?
    scope.recent.includes(:user, :burger, burger: :burger_stat)
  end
end
