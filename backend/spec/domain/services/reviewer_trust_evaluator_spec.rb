require "rails_helper"

RSpec.describe ReviewerTrustEvaluator do
  subject(:evaluator) { described_class.new }

  describe "#call" do
    context "when user has no reviews" do
      it "returns newcomer level" do
        user = create(:user)
        trust = evaluator.call(user)
        expect(trust.level).to eq(:newcomer)
        expect(trust.score).to eq(0.5)
      end
    end

    context "when user has 3 reviews" do
      it "returns regular level" do
        user = create(:user)
        burger = create(:burger)
        create_list(:review, 3, user: user, burger: burger)

        trust = evaluator.call(user)
        expect(trust.level).to eq(:regular)
      end
    end

    context "when user has 20+ reviews" do
      it "returns expert level" do
        user = create(:user)
        burger = create(:burger)
        create_list(:review, 20, user: user, burger: burger)

        trust = evaluator.call(user)
        expect(trust.level).to eq(:expert)
        expect(trust.score).to eq(1.0)
      end
    end

    context "when user always gives the same rating (low variance)" do
      it "applies the variance penalty" do
        user = create(:user)
        burger = create(:burger)
        create_list(:review, 5, user: user, burger: burger, rating: 5)

        trust = evaluator.call(user)
        # base_score for 3-9 reviews = 0.7, with penalty 0.7 × 0.7 = 0.49
        expect(trust.score).to be < 0.7
      end
    end

    context "when user has varied ratings" do
      it "does not apply the variance penalty" do
        user = create(:user)
        burger = create(:burger)
        [1, 2, 4, 5, 3].each do |r|
          create(:review, user: user, burger: burger, rating: r)
        end

        trust = evaluator.call(user)
        expect(trust.score).to eq(0.7)
      end
    end

    it "returns a ReviewerTrust value object" do
      user = create(:user)
      expect(evaluator.call(user)).to be_a(ReviewerTrust)
    end
  end
end
