module Admin
  class ShopsController < ApplicationController
    include Authenticatable

    def index
      authorize Shop, :moderate_index?
      shops = Shops::AdminShopQuery.new.search(status: params[:status])
      render json: shops.map { |s| ShopSerializer.new(s).as_json }
    end

    def update
      shop = shop_repository.find!(params[:id])
      authorize shop, :update?
      param = Shops::UpdateParameter.new(shop_params.to_h.slice("name").symbolize_keys)
      shop = Shops::UpdateShopService.new(shop: shop, params: param).invoke
      render json: ShopSerializer.new(shop).as_json
    rescue ActiveRecord::RecordNotFound
      render json: { error: "Shop not found" }, status: :not_found
    rescue ActiveRecord::RecordInvalid => e
      render json: { errors: e.record.errors.full_messages }, status: :unprocessable_entity
    end

    def approve
      shop = shop_repository.find!(params[:id])
      authorize shop, :approve?
      shop = Shops::ModerateShopService.new(shop: shop).approve
      render json: ShopSerializer.new(shop).as_json
    rescue ActiveRecord::RecordNotFound
      render json: { error: "Shop not found" }, status: :not_found
    end

    def reject
      shop = shop_repository.find!(params[:id])
      authorize shop, :reject?
      param = Shops::RejectParameter.new(moderation_note: params[:moderation_note])
      shop = Shops::ModerateShopService.new(shop: shop).reject(moderation_note: param.moderation_note)
      render json: ShopSerializer.new(shop).as_json
    rescue ActiveRecord::RecordNotFound
      render json: { error: "Shop not found" }, status: :not_found
    end

    private

    def shop_params
      params.require(:shop).permit(:name)
    end

    def shop_repository
      Shops::ShopRepository.new
    end
  end
end
