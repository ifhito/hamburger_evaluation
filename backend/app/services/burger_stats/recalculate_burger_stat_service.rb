module BurgerStats
  class RecalculateBurgerStatService
    def initialize(burger, repository: BurgerStats::BurgerStatRepository.new)
      @burger = burger
      @repository = repository
    end

    def invoke
      projection_input = @repository.projection_input_for(@burger)
      score = Reviews::BurgerScoreCalculator.new.call(projection_input.review_facts)

      @repository.upsert_projection!(
        burger_id:      @burger.id,
        review_count:   projection_input.review_count,
        average_rating: projection_input.average_rating,
        weighted_score: score.weighted_average,
        confidence:     score.confidence,
        calculated_at:  Time.current
      )
    end
  end
end
