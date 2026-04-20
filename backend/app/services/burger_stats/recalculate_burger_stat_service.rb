module BurgerStats
  class RecalculateBurgerStatService
    def initialize(burger)
      @burger = burger
    end

    def invoke
      score  = Reviews::BurgerScoreCalculator.new.call(@burger)
      active = @burger.reviews.kept
      now    = Time.current

      BurgerStat.upsert(
        {
          burger_id:      @burger.id,
          review_count:   active.count,
          average_rating: active.average(:rating).to_f.round(2),
          weighted_score: score.weighted_average,
          confidence:     score.confidence,
          created_at:     now,
          updated_at:     now
        },
        unique_by: :burger_id,
        update_only: %i[review_count average_rating weighted_score confidence updated_at]
      )
    end
  end
end
