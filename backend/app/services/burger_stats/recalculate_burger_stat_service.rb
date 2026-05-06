module BurgerStats
  class RecalculateBurgerStatService
    def initialize(burger, repository: BurgerStats::BurgerStatRepository.new)
      @burger = burger
      @repository = repository
    end

    def invoke
      active_reviews = @repository.active_reviews_for(@burger)
      facts = active_reviews.map { |review| review_fact_for(review) }
      score = Reviews::BurgerScoreCalculator.new.call(facts)

      @repository.upsert_projection!(
        burger_id:      @burger.id,
        review_count:   active_reviews.size,
        average_rating: @repository.average_rating_for(active_reviews),
        weighted_score: score.weighted_average,
        confidence:     score.confidence,
        calculated_at:  Time.current
      )
    end

    private

    def review_fact_for(review)
      Reviews::ReviewFact.new(
        rating: review.rating,
        created_at: review.created_at,
        reviewer_history: reviewer_history_for(review.user)
      )
    end

    def reviewer_history_for(user)
      Reviews::ReviewerHistory.new(ratings: @repository.reviewer_ratings_for(user))
    end
  end
end
