require "rails_helper"

RSpec.describe Reviews::DeleteReviewService do
  let(:review) { create(:review) }

  describe "#invoke" do
    it "レビューをソフトデリートすること" do
      described_class.new(review: review).invoke
      expect(review.reload.discarded_at).not_to be_nil
    end

    it "Review.keptから除外されること" do
      described_class.new(review: review).invoke
      expect(Review.kept).not_to include(review)
    end
  end
end
