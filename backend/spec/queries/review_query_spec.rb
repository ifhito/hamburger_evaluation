require "rails_helper"

RSpec.describe ReviewQuery, type: :query do
  let(:burger) { create(:burger) }

  describe ".call" do
    subject(:result) { ReviewQuery.call(params) }

    context "with no params" do
      let(:params) { {} }

      it "returns all kept reviews" do
        create_list(:review, 3, burger: burger)
        discarded = create(:review, burger: burger)
        discarded.discard
        expect(result.map(&:id)).not_to include(discarded.id)
        expect(result.count).to eq(3)
      end

      it "returns reviews in descending order of created_at" do
        old_review = create(:review, burger: burger, created_at: 2.days.ago)
        new_review = create(:review, burger: burger, created_at: 1.day.ago)
        expect(result.first).to eq(new_review)
        expect(result.last).to eq(old_review)
      end
    end

    context "with rating param" do
      let(:params) { { rating: "5" } }

      it "returns only reviews with rating 5" do
        r5 = create(:review, burger: burger, rating: 5)
        create(:review, burger: burger, rating: 3)
        expect(result).to contain_exactly(r5)
      end
    end

    context "with keyword param" do
      let(:params) { { keyword: "crispy" } }

      it "returns only reviews whose comment matches the keyword" do
        matched = create(:review, burger: burger, comment: "外はcrispy中はジューシー")
        create(:review, burger: burger, comment: "普通のバーガー")
        expect(result).to contain_exactly(matched)
      end
    end

    context "with both rating and keyword params" do
      let(:params) { { rating: "5", keyword: "crispy" } }

      it "applies both filters" do
        matched = create(:review, burger: burger, rating: 5, comment: "crispy texture")
        create(:review, burger: burger, rating: 5, comment: "普通")
        create(:review, burger: burger, rating: 3, comment: "crispy but not five")
        expect(result).to contain_exactly(matched)
      end
    end
  end
end
