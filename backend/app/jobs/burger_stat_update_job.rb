class BurgerStatUpdateJob < ApplicationJob
  queue_as :default

  def perform(burger_id)
    burger = Burger.find(burger_id)
    BurgerStats::RecalculateBurgerStatService.new(burger).invoke
  end
end
