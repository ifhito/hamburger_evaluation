class BurgerStatUpdateJob < ApplicationJob
  queue_as :default

  def perform(burger_id)
    burger = Burger.find(burger_id)
    BurgerStat.recalculate_for(burger)
  end
end
