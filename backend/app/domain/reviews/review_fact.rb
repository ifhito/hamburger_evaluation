module Reviews
  class ReviewFact
    attr_reader :rating, :created_at, :reviewer_history

    def initialize(rating:, created_at:, reviewer_history:)
      @rating = rating.to_f
      @created_at = created_at
      @reviewer_history = reviewer_history
    end
  end
end
