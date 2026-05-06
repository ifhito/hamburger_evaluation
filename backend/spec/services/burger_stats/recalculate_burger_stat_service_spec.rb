require "rails_helper"

RSpec.describe BurgerStats::RecalculateBurgerStatService do
  describe "#invoke" do
    it "uses a repository for review loading and projection persistence" do
      burger = instance_double(Burger, id: 123)
      reviewer = instance_double(User)
      review = instance_double(Review, rating: 4, created_at: Time.current, user: reviewer)
      repository = instance_double(BurgerStats::BurgerStatRepository)

      allow(repository).to receive(:active_reviews_for).with(burger).and_return([ review ])
      allow(repository).to receive(:reviewer_ratings_for).with(reviewer).and_return([ 4, 5, 3 ])
      allow(repository).to receive(:average_rating_for).with([ review ]).and_return(4.0)
      allow(repository).to receive(:upsert_projection!)

      described_class.new(burger, repository: repository).invoke

      expect(repository).to have_received(:upsert_projection!).with(
        hash_including(
          burger_id: 123,
          review_count: 1,
          average_rating: 4.0,
          weighted_score: be_a(Float),
          confidence: be_a(Float)
        )
      )
    end
  end
end
