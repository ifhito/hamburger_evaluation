require "rails_helper"

RSpec.describe ReviewerTrust do
  describe "#initialize" do
    it "clamps score between 0.0 and 1.0" do
      trust = ReviewerTrust.new(score: 1.5, level: :expert)
      expect(trust.score).to eq(1.0)
    end

    it "is frozen after initialization" do
      expect(ReviewerTrust.new(score: 0.8, level: :regular)).to be_frozen
    end
  end

  describe "#to_f" do
    it "returns the score as Float" do
      trust = ReviewerTrust.new(score: 0.7, level: :regular)
      expect(trust.to_f).to eq(0.7)
    end
  end

  describe "#to_s" do
    it "includes the level and score percentage" do
      trust = ReviewerTrust.new(score: 0.7, level: :regular)
      expect(trust.to_s).to include("regular")
      expect(trust.to_s).to include("70.0%")
    end
  end

  describe "LEVELS" do
    it "defines levels with min_reviews and base_score" do
      expect(ReviewerTrust::LEVELS[:newcomer][:min_reviews]).to eq(0)
      expect(ReviewerTrust::LEVELS[:expert][:base_score]).to eq(1.0)
    end
  end
end
