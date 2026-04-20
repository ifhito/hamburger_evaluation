module Reviews
  class ReviewerTrust
    LEVELS = {
      newcomer: { min_reviews: 0,  base_score: 0.5 },
      regular:  { min_reviews: 3,  base_score: 0.7 },
      veteran:  { min_reviews: 10, base_score: 0.9 },
      expert:   { min_reviews: 20, base_score: 1.0 }
    }.freeze

    attr_reader :score, :level

    def initialize(score:, level:)
      @score = score.clamp(0.0, 1.0)
      @level = level
      freeze
    end

    def to_f = score
    def to_s = "#{level}(#{(score * 100).round(1)}%)"
  end
end
