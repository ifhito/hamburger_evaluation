module Reviews
  class ReviewEvents
    def self.review_changed_for_burger(burger_id)
      BurgerStatUpdateJob.perform_later(burger_id)
    end
  end
end
