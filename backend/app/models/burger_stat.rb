class BurgerStat < ApplicationRecord
  belongs_to :burger

  def self.recalculate_for(burger)
    active = burger.reviews.kept
    find_or_initialize_by(burger: burger).update!(
      review_count:   active.count,
      average_rating: active.average(:rating).to_f.round(2)
    )
  end
end
