module Shops
  class CreateShopService
    def initialize(user:, params:, repository: Shops::ShopRepository.new)
      @user       = user
      @params     = params
      @repository = repository
    end

    def invoke
      @repository.create!(
        creator: @user,
        name:    @params.name,
        status:  Shops::ShopStatus.initial.to_s
      )
    end
  end
end
