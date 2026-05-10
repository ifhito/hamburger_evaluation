require "rails_helper"

RSpec.describe BurgerStats::BurgerStatRepository do
  subject(:repository) { described_class.new }

  describe "#find_burger!" do
    it "returns the burger" do
      burger = create(:burger)

      expect(repository.find_burger!(burger.id)).to eq(burger)
    end
  end

  describe "#projection_input_for" do
    it "returns review facts, kept review count, and average rating for the burger" do
      user = create(:user)
      burger = create(:burger)
      create(:review, user: user, burger: burger, rating: 5)
      create(:review, user: user, burger: burger, rating: 3)
      discarded_review = create(:review, user: user, burger: burger, rating: 1)
      discarded_review.discard

      input = repository.projection_input_for(burger)

      expect(input).to be_a(BurgerStats::ReviewProjectionInput)
      expect(input.review_facts.map(&:rating)).to contain_exactly(5.0, 3.0)
      expect(input.review_count).to eq(2)
      expect(input.average_rating).to eq(4.0)
      expect(input.review_facts.map(&:reviewer_history).map(&:ratings)).to all(eq([ 5.0, 3.0 ]))
    end

    it "returns zero summary values when there are no active reviews" do
      burger = create(:burger)

      input = repository.projection_input_for(burger)

      expect(input.review_facts).to eq([])
      expect(input.review_count).to eq(0)
      expect(input.average_rating).to eq(0.0)
    end
  end

  describe "#upsert_projection!" do
    it "creates or updates the burger stat projection" do
      burger = create(:burger)

      repository.upsert_projection!(
        burger_id: burger.id,
        review_count: 2,
        average_rating: 4.5,
        weighted_score: 4.2,
        confidence: 0.8,
        calculated_at: Time.current
      )

      stat = BurgerStat.find_by!(burger_id: burger.id)
      expect(stat.review_count).to eq(2)
      expect(stat.average_rating).to eq(4.5)
      expect(stat.weighted_score).to eq(4.2)
      expect(stat.confidence).to eq(0.8)
    end
  end
end
