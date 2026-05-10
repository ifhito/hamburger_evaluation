module Reviews
  class DeleteReviewService
    def initialize(review:, repository: Reviews::ReviewRepository.new)
      @review = review
      @repository = repository
    end

    def invoke
      @repository.discard!(@review)
    end
  end
end
