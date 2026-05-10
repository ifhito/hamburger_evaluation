module BurgerStats
  class ReviewProjectionInput
    attr_reader :review_facts, :review_count, :average_rating

    def initialize(review_facts:, review_count:, average_rating:)
      @review_facts = review_facts.freeze
      @review_count = review_count
      @average_rating = average_rating
      freeze
    end
  end
end
