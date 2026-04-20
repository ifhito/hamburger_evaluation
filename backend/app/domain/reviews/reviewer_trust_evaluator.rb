module Reviews
  class ReviewerTrustEvaluator
    LOW_VARIANCE_THRESHOLD = 0.3
    LOW_VARIANCE_PENALTY   = 0.7

    def call(user)
      reviews = user.reviews.kept
      base    = base_score(reviews.count)
      factor  = variance_factor(reviews)
      level   = determine_level(reviews.count)

      Reviews::ReviewerTrust.new(score: base * factor, level: level)
    end

    private

    def base_score(count)
      Reviews::ReviewerTrust::LEVELS
        .select { |_, v| count >= v[:min_reviews] }
        .values.last[:base_score]
    end

    def variance_factor(reviews)
      return 1.0 if reviews.count < 3
      ratings = reviews.pluck(:rating).map(&:to_f)
      mean    = ratings.sum / ratings.size
      var     = ratings.sum { |r| (r - mean)**2 } / ratings.size
      var < LOW_VARIANCE_THRESHOLD ? LOW_VARIANCE_PENALTY : 1.0
    end

    def determine_level(count)
      Reviews::ReviewerTrust::LEVELS
        .select { |_, v| count >= v[:min_reviews] }
        .keys.last
    end
  end
end
