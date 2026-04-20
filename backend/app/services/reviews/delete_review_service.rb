module Reviews
  class DeleteReviewService
    def initialize(review:)
      @review = review
    end

    def invoke
      @review.discard
    end
  end
end
