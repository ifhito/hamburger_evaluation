class ShopsController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index, :show ]
  before_action :current_user_if_present, only: [ :index, :show ]

  def index
    shops = Shops::ShopQuery.new.search(keyword: params[:keyword], viewer: current_user)
    render json: shops.map { |s| { id: s.id, name: s.name, status: s.status } }
  end

  def show
    shop = Shops::ShopQuery.new.find_visible!(params[:id], viewer: current_user)
    reviews = Reviews::ReviewQuery.new({ shop_id: shop.id }).search
    render json: ShopSerializer.new(shop).as_json.merge(
      reviews: reviews.map { |r| ReviewSerializer.new(r).as_json }
    )
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Shop not found" }, status: :not_found
  end

  def create
    authorize Shop, :create?
    param = Shops::CreateParameter.new(name: shop_params[:name])
    shop = Shops::CreateShopService.new(user: current_user, params: param).invoke
    render json: ShopSerializer.new(shop).as_json, status: :created
  rescue ActiveRecord::RecordInvalid => e
    render json: { errors: e.record.errors.full_messages }, status: :unprocessable_entity
  end

  private

  def shop_params
    params.require(:shop).permit(:name)
  end
end
