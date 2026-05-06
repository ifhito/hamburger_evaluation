require "rails_helper"

RSpec.describe BurgerStats::BurgerStatRepository do
  subject(:repository) { described_class.new }

  describe "#active_reviews_for" do
    it "loads kept reviews with users" do
      burger = create(:burger)
      kept_review = create(:review, burger: burger)
      discarded_review = create(:review, burger: burger)
      discarded_review.discard

      expect(repository.active_reviews_for(burger)).to contain_exactly(kept_review)
    end
  end

  describe "#reviewer_ratings_for" do
    it "returns kept review ratings for the reviewer" do
      user = create(:user)
      burger = create(:burger)
      create(:review, user: user, burger: burger, rating: 5)
      discarded_review = create(:review, user: user, burger: burger, rating: 1)
      discarded_review.discard

      expect(repository.reviewer_ratings_for(user)).to eq([ 5 ])
    end
  end

  describe "#average_rating_for" do
    it "returns a rounded average rating" do
      reviews = [ instance_double(Review, rating: 4), instance_double(Review, rating: 5) ]

      expect(repository.average_rating_for(reviews)).to eq(4.5)
    end

    it "returns 0.0 when there are no reviews" do
      expect(repository.average_rating_for([])).to eq(0.0)
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
