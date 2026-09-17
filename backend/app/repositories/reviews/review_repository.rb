module Reviews
  class ReviewRepository
    def find_kept!(id)
      Review.kept.find(id)
    end

    def update!(review, **attrs)
      review.update!(attrs)
      review
    end

    def discard!(review)
      review.discard
    end
  end
end
