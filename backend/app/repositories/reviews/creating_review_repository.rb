module Reviews
  class CreatingReviewRepository
    def transaction(&block)
      ApplicationRecord.transaction(&block)
    end

    def find_shop!(shop_id)
      Shop.find(shop_id)
    end

    def with_shop_lock(shop, &block)
      shop.with_lock(&block)
    end

    def find_burger_for_shop(shop:, burger_name:)
      shop.burgers.find_by(name: burger_name)
    end

    def create_burger_for_shop(shop:, burger_name:)
      Burger.create!(shop: shop, name: burger_name)
    end

    def create_review!(user:, burger:, rating:, comment:)
      user.reviews.create!(rating: rating, comment: comment, burger: burger)
    end
  end
end
