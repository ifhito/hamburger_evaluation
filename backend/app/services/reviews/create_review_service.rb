module Reviews
  class CreateReviewService
    def initialize(user:, params:, repository: Reviews::CreatingReviewRepository.new)
      @user       = user
      @params     = params
      @repository = repository
    end

    def invoke
      ApplicationRecord.transaction do
        shop   = Shop.find(@params.shop_id)
        burger = shop.with_lock do
          Reviews::BurgerFinder.new.find_for_shop(shop, @params.burger_name) ||
            @repository.create_burger_for_shop(shop, @params.burger_name)
        end
        review = @repository.save!(
          user:    @user,
          burger:  burger,
          rating:  @params.rating,
          comment: @params.comment
        )
        review.burger.reload
        review
      end
    end
  end
end
