require "rails_helper"

RSpec.describe Reviews::DeleteReviewService do
  let(:review) { create(:review) }

  describe "#invoke" do
    context "永続化依存を注入した場合" do
      it "repositoryにレビュー削除を委譲すること" do
        repository = instance_double(Reviews::ReviewRepository)
        allow(repository).to receive(:discard!).with(review).and_return(true)

        described_class.new(review: review, repository: repository).invoke

        expect(repository).to have_received(:discard!).with(review)
      end
    end

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
