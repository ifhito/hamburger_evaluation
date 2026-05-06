module Reviews
  class ReviewerTrustEvaluator
    LOW_VARIANCE_THRESHOLD = 0.3
    LOW_VARIANCE_PENALTY   = 0.7

    def call(history)
      base   = base_score(history.review_count)
      factor = variance_factor(history.ratings)
      level  = determine_level(history.review_count)

      Reviews::ReviewerTrust.new(score: base * factor, level: level)
    end

    private

    def base_score(count)
      Reviews::ReviewerTrust::LEVELS
        .select { |_, v| count >= v[:min_reviews] }
        .values.last[:base_score]
    end

    def variance_factor(ratings)
      return 1.0 if ratings.size < 3

      mean = ratings.sum / ratings.size
      variance = ratings.sum { |rating| (rating - mean)**2 } / ratings.size
      variance < LOW_VARIANCE_THRESHOLD ? LOW_VARIANCE_PENALTY : 1.0
    end

    def determine_level(count)
      Reviews::ReviewerTrust::LEVELS
        .select { |_, v| count >= v[:min_reviews] }
        .keys.last
    end
  end
end
