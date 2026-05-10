class ShopsController < ApplicationController
  def index
    shops = Shops::ShopQuery.new.search(keyword: params[:keyword])
    render json: shops.map { |s| shop_json(s) }
  end

  def show
    shop = Shops::ShopQuery.new.find!(params[:id])
    reviews = Reviews::ReviewQuery.new({ shop_id: shop.id }).search
    render json: shop_json(shop).merge(
      reviews: reviews.map { |r| ReviewSerializer.new(r).as_json }
    )
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Shop not found" }, status: :not_found
  end

  private

  def shop_json(shop)
    {
      id: shop.id,
      name: shop.name,
      stat: shop_stat_json(shop.shop_stat)
    }
  end

  def shop_stat_json(stat)
    return { average_rating: 0.0, weighted_score: 0.0, burger_count: 0, review_count: 0, confidence: 0.0 } if stat.nil?

    {
      average_rating: stat.average_rating,
      weighted_score: stat.weighted_score,
      burger_count: stat.burger_count,
      review_count: stat.review_count,
      confidence: stat.confidence
    }
  end
end
