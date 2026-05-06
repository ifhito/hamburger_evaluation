require "rails_helper"

RSpec.describe Reviews::BurgerScoreCalculator do
  subject(:calculator) { described_class.new }

  def history(ratings)
    Reviews::ReviewerHistory.new(ratings: ratings)
  end

  def review_fact(rating:, created_at: Time.current, reviewer_ratings: [ rating ])
    Reviews::ReviewFact.new(
      rating: rating,
      created_at: created_at,
      reviewer_history: history(reviewer_ratings)
    )
  end

  describe "#call" do
    context "when there are no reviews" do
      it "returns BurgerScore.empty" do
        score = calculator.call([])
        expect(score.weighted_average).to eq(0.0)
        expect(score.confidence).to eq(0.0)
        expect(score.sample_size).to eq(0)
      end
    end

    context "when there are reviews" do
      it "returns a Reviews::BurgerScore value object" do
        expect(calculator.call([ review_fact(rating: 4) ])).to be_a(Reviews::BurgerScore)
      end

      it "reflects the ratings in weighted_average" do
        score = calculator.call([ review_fact(rating: 5) ])
        expect(score.weighted_average).to be > 0.0
      end
    end

    context "when reviewer trust differs" do
      it "weights expert reviewers more than newcomers" do
        facts = [
          review_fact(rating: 1, reviewer_ratings: [ 1 ]),
          review_fact(rating: 5, reviewer_ratings: Array.new(20, 3) + [ 5 ])
        ]

        score = calculator.call(facts)
        expect(score.weighted_average).to be > 3.0
      end
    end

    context "with injected trust_evaluator (stub)" do
      it "uses the injected evaluator" do
        stub_trust = instance_double(Reviews::ReviewerTrustEvaluator)
        allow(stub_trust).to receive(:call).and_return(
          Reviews::ReviewerTrust.new(score: 1.0, level: :expert)
        )

        calculator = Reviews::BurgerScoreCalculator.new(trust_evaluator: stub_trust)

        expect(stub_trust).to receive(:call).at_least(:once)
        calculator.call([ review_fact(rating: 4) ])
      end
    end
  end
end
