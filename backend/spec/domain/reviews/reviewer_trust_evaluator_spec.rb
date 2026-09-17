require "rails_helper"

RSpec.describe Reviews::ReviewerTrustEvaluator do
  subject(:evaluator) { described_class.new }

  describe "#call" do
    context "when reviewer has no reviews" do
      it "returns newcomer level" do
        history = Reviews::ReviewerHistory.new(ratings: [])
        trust = evaluator.call(history)
        expect(trust.level).to eq(:newcomer)
        expect(trust.score).to eq(0.5)
      end
    end

    context "when reviewer has 3 reviews" do
      it "returns regular level" do
        history = Reviews::ReviewerHistory.new(ratings: [ 4, 4, 5 ])

        trust = evaluator.call(history)
        expect(trust.level).to eq(:regular)
      end
    end

    context "when reviewer has 20+ reviews" do
      it "returns expert level" do
        history = Reviews::ReviewerHistory.new(ratings: Array.new(20, 4))

        trust = evaluator.call(history)
        expect(trust.level).to eq(:expert)
        expect(trust.score).to eq(0.7)
      end
    end

    context "when reviewer always gives the same rating (low variance)" do
      it "applies the variance penalty" do
        history = Reviews::ReviewerHistory.new(ratings: [ 5, 5, 5, 5, 5 ])

        trust = evaluator.call(history)
        expect(trust.score).to be < 0.7
      end
    end

    context "when reviewer has varied ratings" do
      it "does not apply the variance penalty" do
        history = Reviews::ReviewerHistory.new(ratings: [ 1, 2, 4, 5, 3 ])

        trust = evaluator.call(history)
        expect(trust.score).to eq(0.7)
      end
    end

    it "returns a Reviews::ReviewerTrust value object" do
      history = Reviews::ReviewerHistory.new(ratings: [])
      expect(evaluator.call(history)).to be_a(Reviews::ReviewerTrust)
    end
  end
end
