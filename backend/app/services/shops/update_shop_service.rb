module Shops
  class UpdateShopService
    def initialize(shop:, params:, repository: Shops::ShopRepository.new)
      @shop       = shop
      @params     = params
      @repository = repository
    end

    def invoke
      attrs = @params.to_h.slice(:name)
      @repository.update!(@shop, **attrs) unless attrs.empty?
      @shop
    end
  end
end
