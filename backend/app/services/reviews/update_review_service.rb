module Reviews
  class UpdateReviewService
    def initialize(review:, params:, repository: Reviews::ReviewRepository.new)
      @review = review
      @params = params
      @repository = repository
    end

    def invoke
      attrs = { rating: @params.rating, comment: @params.comment }.compact
      @repository.update!(@review, **attrs) unless attrs.empty?
      @review
    end
  end
end
