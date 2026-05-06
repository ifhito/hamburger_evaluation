class ShopsController < ApplicationController
  def index
    shops = Shops::ShopQuery.new.search(keyword: params[:keyword])
    render json: shops.map { |s| { id: s.id, name: s.name } }
  end

  def show
    shop = Shops::ShopQuery.new.find!(params[:id])
    reviews = Reviews::ReviewQuery.new({ shop_id: shop.id }).search
    render json: {
      id:      shop.id,
      name:    shop.name,
      reviews: reviews.map { |r| ReviewSerializer.new(r).as_json }
    }
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Shop not found" }, status: :not_found
  end
end
