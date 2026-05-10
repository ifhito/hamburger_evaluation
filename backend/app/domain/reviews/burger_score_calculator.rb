module Reviews
  class BurgerScoreCalculator
    RECENCY_HALF_LIFE_DAYS = 180.0

    def initialize(trust_evaluator: Reviews::ReviewerTrustEvaluator.new)
      @trust_evaluator = trust_evaluator
    end

    def call(review_facts)
      facts = review_facts.to_a
      return Reviews::BurgerScore.empty if facts.empty?

      weighted = facts.map { |fact| [ fact.rating, weight_for(fact) ] }
      total_weight = weighted.sum(&:last)
      weighted_average = weighted.sum { |rating, weight| rating * weight } / total_weight
      confidence = calculate_confidence(facts.size, total_weight)

      Reviews::BurgerScore.new(
        weighted_average: weighted_average,
        confidence:       confidence,
        sample_size:      facts.size
      )
    end

    private

    def weight_for(fact)
      @trust_evaluator.call(fact.reviewer_history).to_f * recency_factor(fact.created_at)
    end

    def recency_factor(created_at)
      days_ago = (Time.current - created_at) / 1.day
      Math.exp(-days_ago * Math.log(2) / RECENCY_HALF_LIFE_DAYS)
    end

    def calculate_confidence(count, total_weight)
      review_factor = [ count / 10.0, 1.0 ].min
      weight_factor = [ total_weight / count.to_f, 1.0 ].min
      (review_factor * 0.6 + weight_factor * 0.4).clamp(0.0, 1.0)
    end
  end
end
