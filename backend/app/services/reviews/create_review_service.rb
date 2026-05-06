module Reviews
  class CreateReviewService
    def initialize(user:, params:, repository: Reviews::CreatingReviewRepository.new)
      @user       = user
      @params     = params
      @repository = repository
    end

    def invoke
      @repository.transaction do
        shop = @repository.find_shop!(@params.shop_id)
        burger = @repository.with_shop_lock(shop) do
          @repository.find_burger_for_shop(shop: shop, burger_name: @params.burger_name) ||
            @repository.create_burger_for_shop(shop: shop, burger_name: @params.burger_name)
        end
        @repository.create_review!(
          user:    @user,
          burger:  burger,
          rating:  @params.rating,
          comment: @params.comment
        )
      end
    end
  end
end
