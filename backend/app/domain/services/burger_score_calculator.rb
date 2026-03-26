class BurgerScoreCalculator
  RECENCY_HALF_LIFE_DAYS = 180.0

  def initialize(trust_evaluator: ReviewerTrustEvaluator.new)
    @trust_evaluator = trust_evaluator
  end

  def call(burger)
    reviews = burger.reviews.kept.includes(:user)
    return BurgerScore.empty if reviews.empty?

    weighted     = reviews.map { |r| [r.rating.to_f, weight_for(r)] }
    total_weight = weighted.sum(&:last)
    weighted_avg = weighted.sum { |rating, w| rating * w } / total_weight
    confidence   = calculate_confidence(reviews.count, total_weight)

    BurgerScore.new(
      weighted_average: weighted_avg,
      confidence:       confidence,
      sample_size:      reviews.count
    )
  end

  private

  def weight_for(review)
    @trust_evaluator.call(review.user).to_f * recency_factor(review.created_at)
  end

  def recency_factor(created_at)
    days_ago = (Time.current - created_at) / 1.day
    Math.exp(-days_ago * Math.log(2) / RECENCY_HALF_LIFE_DAYS)
  end

  def calculate_confidence(count, total_weight)
    review_factor = [count / 10.0, 1.0].min
    weight_factor = [total_weight / count.to_f, 1.0].min
    (review_factor * 0.6 + weight_factor * 0.4).clamp(0.0, 1.0)
  end
end
