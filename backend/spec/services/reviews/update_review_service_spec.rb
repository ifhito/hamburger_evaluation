require "rails_helper"

RSpec.describe Reviews::UpdateReviewService do
  let(:review) { create(:review, rating: 3, comment: "Original comment") }

  describe "#invoke" do
    context "ratingとcommentを両方更新する場合" do
      let(:params) { Reviews::UpdateParameter.new(rating: 5, comment: "Updated comment") }

      it "ratingとcommentが更新されること" do
        described_class.new(review: review, params: params).invoke
        expect(review.reload.rating).to eq(5)
        expect(review.reload.comment).to eq("Updated comment")
      end
    end

    context "commentのみ更新する場合（部分更新）" do
      let(:params) { Reviews::UpdateParameter.new(comment: "Partial update") }

      it "commentだけ更新されratingは変わらないこと" do
        described_class.new(review: review, params: params).invoke
        expect(review.reload.comment).to eq("Partial update")
        expect(review.reload.rating).to eq(3)
      end
    end

    context "何も渡されない場合" do
      let(:params) { Reviews::UpdateParameter.new({}) }

      it "レコードが変更されないこと" do
        expect {
          described_class.new(review: review, params: params).invoke
        }.not_to change { review.reload.updated_at }
      end
    end

    context "更新後のレビューを返すこと" do
      let(:params) { Reviews::UpdateParameter.new(rating: 4) }

      it "更新済みのreviewオブジェクトを返すこと" do
        result = described_class.new(review: review, params: params).invoke
        expect(result).to eq(review)
      end
    end
  end
end
