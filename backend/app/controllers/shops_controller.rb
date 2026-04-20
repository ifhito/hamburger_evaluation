class ShopsController < ApplicationController
  def index
    shops = Shop.all
    shops = shops.where("name ILIKE ?", "%#{params[:keyword]}%") if params[:keyword].present?
    render json: shops.order(:name).map { |s| { id: s.id, name: s.name } }
  end

  def show
    shop = Shop.find(params[:id])
    reviews = Reviews::ReviewFinder.new({ shop_id: shop.id }).search
    render json: {
      id:      shop.id,
      name:    shop.name,
      reviews: reviews.map { |r| ReviewSerializer.new(r).as_json }
    }
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Shop not found" }, status: :not_found
  end
end
