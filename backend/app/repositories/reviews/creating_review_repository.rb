module Reviews
  class CreatingReviewRepository
    def create_burger_for_shop(shop, burger_name)
      burger = Burger.create!(name: burger_name)
      ShopsAndBurger.create!(shop: shop, burger: burger)
      burger
    end

    def save!(user:, burger:, rating:, comment:)
      user.reviews.create!(rating: rating, comment: comment, burger: burger)
    end
  end
end
