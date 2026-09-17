module Shops
  class ShopRepository
    def find!(id)
      Shop.find(id)
    end

    def create!(creator:, name:, status:)
      Shop.create!(creator: creator, name: name, status: status)
    end

    def update!(shop, **attrs)
      shop.update!(attrs)
      shop
    end

    def update_status!(shop, status:, moderation_note: nil)
      shop.update!(status: status, moderation_note: moderation_note)
      shop
    end
  end
end
