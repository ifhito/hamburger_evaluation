require "rails_helper"

RSpec.describe BurgerScore do
  describe "#initialize" do
    it "rounds weighted_average to 2 decimal places" do
      score = BurgerScore.new(weighted_average: 3.14159, confidence: 0.8, sample_size: 5)
      expect(score.weighted_average).to eq(3.14)
    end

    it "clamps confidence between 0.0 and 1.0" do
      score = BurgerScore.new(weighted_average: 4.0, confidence: 1.5, sample_size: 10)
      expect(score.confidence).to eq(1.0)
    end

    it "is frozen after initialization" do
      score = BurgerScore.new(weighted_average: 4.0, confidence: 0.8, sample_size: 5)
      expect(score).to be_frozen
    end
  end

  describe "#reliable?" do
    it "returns true when confidence >= 0.6" do
      score = BurgerScore.new(weighted_average: 4.0, confidence: 0.6, sample_size: 10)
      expect(score.reliable?).to be true
    end

    it "returns false when confidence < 0.6" do
      score = BurgerScore.new(weighted_average: 4.0, confidence: 0.3, sample_size: 1)
      expect(score.reliable?).to be false
    end
  end

  describe ".empty" do
    it "returns a BurgerScore with zero values" do
      score = BurgerScore.empty
      expect(score.weighted_average).to eq(0.0)
      expect(score.confidence).to eq(0.0)
      expect(score.sample_size).to eq(0)
    end
  end
end
