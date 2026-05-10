class ReviewSerializer
  def initialize(review)
    @review = review
  end

  def as_json
    {
      id:         @review.id,
      rating:     @review.rating,
      comment:    @review.comment,
      created_at: @review.created_at,
      user:       user_json,
      burger:     burger_json
    }
  end

  private

  def user_json
    return nil unless @review.user
    { id: @review.user.id, username: @review.user.username }
  end

  def burger_json
    return nil unless @review.burger
    stat = @review.burger.burger_stat
    {
      id:             @review.burger.id,
      name:           @review.burger.name,
      average_rating: stat&.average_rating  || 0.0,
      review_count:   stat&.review_count    || 0,
      weighted_score: stat&.weighted_score  || 0.0,
      confidence:     stat&.confidence      || 0.0
    }
  end
end
