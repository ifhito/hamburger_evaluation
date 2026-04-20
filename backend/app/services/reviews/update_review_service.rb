module Reviews
  class UpdateReviewService
    def initialize(review:, params:)
      @review = review
      @params = params
    end

    def invoke
      attrs = { rating: @params.rating, comment: @params.comment }.compact
      @review.update!(attrs) unless attrs.empty?
      @review
    end
  end
end
