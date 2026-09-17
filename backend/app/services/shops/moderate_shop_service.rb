module Shops
  class ModerateShopService
    def initialize(shop:, repository: Shops::ShopRepository.new)
      @shop       = shop
      @repository = repository
    end

    def approve
      next_status = Shops::ShopEntity.from_record(@shop).approve
      @repository.update_status!(@shop, status: next_status.to_s, moderation_note: nil)
    end

    def reject(moderation_note: nil)
      next_status = Shops::ShopEntity.from_record(@shop).reject
      @repository.update_status!(@shop, status: next_status.to_s, moderation_note: moderation_note)
    end
  end
end
