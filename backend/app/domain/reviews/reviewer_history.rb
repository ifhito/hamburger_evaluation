module Reviews
  class ReviewerHistory
    attr_reader :ratings

    def initialize(ratings:)
      @ratings = ratings.map(&:to_f).freeze
    end

    def review_count
      ratings.size
    end
  end
end
