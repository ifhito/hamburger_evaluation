require "rails_helper"

RSpec.describe Burgers::BurgerEntity do
  def fact(rating)
    Reviews::ReviewFact.new(
      rating: rating,
      created_at: Time.current,
      reviewer_history: Reviews::ReviewerHistory.new(ratings: [ rating ])
    )
  end

  describe "#review_count" do
    it "review_facts の件数を返す" do
      entity = described_class.new(id: 1, review_facts: [ fact(4), fact(5) ])
      expect(entity.review_count).to eq(2)
    end
  end

  describe "#average_rating" do
    it "facts の平均を小数2桁で返す" do
      entity = described_class.new(id: 1, review_facts: [ fact(4), fact(5) ])
      expect(entity.average_rating).to eq(4.5)
    end

    it "facts が空なら 0.0" do
      entity = described_class.new(id: 1, review_facts: [])
      expect(entity.average_rating).to eq(0.0)
    end
  end

  describe "#score" do
    it "計算は BurgerScoreCalculator に委譲する" do
      facts = [ fact(4) ]
      calculator = instance_double(Reviews::BurgerScoreCalculator)
      sentinel = Reviews::BurgerScore.empty
      allow(calculator).to receive(:call).with(facts).and_return(sentinel)

      entity = described_class.new(id: 1, review_facts: facts)
      expect(entity.score(calculator: calculator)).to be(sentinel)
      expect(calculator).to have_received(:call).with(facts)
    end

    it "既定では実際の BurgerScore を返す" do
      entity = described_class.new(id: 1, review_facts: [ fact(5) ])
      expect(entity.score).to be_a(Reviews::BurgerScore)
    end
  end
end
