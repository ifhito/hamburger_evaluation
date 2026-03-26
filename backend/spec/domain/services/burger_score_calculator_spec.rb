require "rails_helper"

RSpec.describe BurgerScoreCalculator do
  subject(:calculator) { described_class.new }

  describe "#call" do
    context "when burger has no reviews" do
      it "returns BurgerScore.empty" do
        burger = create(:burger)
        score = calculator.call(burger)
        expect(score.weighted_average).to eq(0.0)
        expect(score.confidence).to eq(0.0)
        expect(score.sample_size).to eq(0)
      end
    end

    context "when burger has reviews" do
      it "returns a BurgerScore value object" do
        burger = create(:burger)
        create(:review, burger: burger, rating: 4)
        expect(calculator.call(burger)).to be_a(BurgerScore)
      end

      it "reflects the ratings in weighted_average" do
        burger = create(:burger)
        create(:review, burger: burger, rating: 5)
        score = calculator.call(burger)
        expect(score.weighted_average).to be > 0.0
      end
    end

    context "when reviewer trust differs" do
      it "weights expert reviewers more than newcomers" do
        burger = create(:burger)

        # newcomer giving rating 1
        newcomer = create(:user)
        create(:review, user: newcomer, burger: burger, rating: 1)

        # expert giving rating 5 (20+ reviews on other burgers)
        expert = create(:user)
        other_burger = create(:burger)
        create_list(:review, 20, user: expert, burger: other_burger, rating: 3)
        create(:review, user: expert, burger: burger, rating: 5)

        score = calculator.call(burger)

        # expert's rating 5 should outweigh newcomer's rating 1
        expect(score.weighted_average).to be > 3.0
      end
    end

    context "with injected trust_evaluator (stub)" do
      it "uses the injected evaluator" do
        stub_trust = instance_double(ReviewerTrustEvaluator)
        allow(stub_trust).to receive(:call).and_return(
          ReviewerTrust.new(score: 1.0, level: :expert)
        )

        calculator = BurgerScoreCalculator.new(trust_evaluator: stub_trust)
        burger = create(:burger)
        create(:review, burger: burger, rating: 4)

        expect(stub_trust).to receive(:call).at_least(:once)
        calculator.call(burger)
      end
    end

    context "when reviews are discarded" do
      it "ignores discarded reviews" do
        burger = create(:burger)
        review = create(:review, burger: burger, rating: 1)
        review.discard

        score = calculator.call(burger)
        expect(score.sample_size).to eq(0)
      end
    end
  end
end
