require "rails_helper"

RSpec.describe BurgerStats::RecalculateBurgerStatService do
  describe "#invoke" do
    it "uses a repository projection input instead of reading review records directly" do
      burger = instance_double(Burger, id: 123)
      facts = [
        Reviews::ReviewFact.new(
          rating: 4,
          created_at: Time.current,
          reviewer_history: Reviews::ReviewerHistory.new(ratings: [ 4, 5, 3 ])
        )
      ]
      projection_input = BurgerStats::ReviewProjectionInput.new(
        review_facts: facts,
        review_count: 1,
        average_rating: 4.0
      )
      repository = instance_double(BurgerStats::BurgerStatRepository)

      allow(repository).to receive(:projection_input_for).with(burger).and_return(projection_input)
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
