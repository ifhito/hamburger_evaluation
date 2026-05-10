class BurgerStatUpdateJob < ApplicationJob
  queue_as :default

  def perform(burger_id)
    burger = BurgerStats::BurgerStatRepository.new.find_burger!(burger_id)
    BurgerStats::RecalculateBurgerStatService.new(burger).invoke
    ShopStats::RecalculateShopStatService.new(burger.shop).invoke
  end
end
