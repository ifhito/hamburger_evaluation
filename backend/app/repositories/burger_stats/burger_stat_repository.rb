module BurgerStats
  class BurgerStatRepository
    def find_burger!(burger_id)
      Burger.find(burger_id)
    end

    def active_reviews_for(burger)
      burger.reviews.kept.includes(:user).to_a
    end

    def reviewer_ratings_for(user)
      user.reviews.kept.map(&:rating)
    end

    def upsert_projection!(burger_id:, review_count:, average_rating:, weighted_score:, confidence:, calculated_at:)
      BurgerStat.upsert(
        {
          burger_id:      burger_id,
          review_count:   review_count,
          average_rating: average_rating,
          weighted_score: weighted_score,
          confidence:     confidence,
          created_at:     calculated_at,
          updated_at:     calculated_at
        },
        unique_by: :burger_id,
        update_only: %i[review_count average_rating weighted_score confidence updated_at],
        record_timestamps: false
      )
    end
  end
end
