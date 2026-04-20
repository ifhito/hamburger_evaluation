class BurgerStat < ApplicationRecord
  belongs_to :burger

  def self.recalculate_for(burger)
    BurgerStats::RecalculateBurgerStatService.new(burger).invoke
  end
end
