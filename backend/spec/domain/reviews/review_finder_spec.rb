require "rails_helper"

RSpec.describe Reviews::ReviewFinder do
  let(:burger) { create(:burger) }

  describe "#search" do
    subject(:result) { described_class.new(params).search }

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

      it "is case-insensitive" do
        matched = create(:review, burger: burger, comment: "CRISPY texture")
        create(:review, burger: burger, comment: "soft bun")
        expect(described_class.new({ keyword: "crispy" }).search).to contain_exactly(matched)
      end
    end

    context "with LIKE wildcard characters in keyword param" do
      it "treats % as a literal character, not a wildcard" do
        create(:review, burger: burger, comment: "great burger")
        create(:review, burger: burger, comment: "50% beef")
        result = described_class.new({ keyword: "%" }).search
        expect(result.map(&:comment)).to contain_exactly("50% beef")
      end

      it "treats _ as a literal character, not a single-char wildcard" do
        create(:review, burger: burger, comment: "great burger")
        has_underscore = create(:review, burger: burger, comment: "my_burger")
        result = described_class.new({ keyword: "_" }).search
        expect(result).to contain_exactly(has_underscore)
      end
    end

    context "with shop_id param" do
      it "returns only reviews for burgers at the given shop" do
        shop = create(:shop)
        sab  = create(:shops_and_burger, shop: shop, burger: burger)
        review_in_shop  = create(:review, burger: burger)
        other_burger    = create(:burger)
        review_other    = create(:review, burger: other_burger)
        expect(described_class.new({ shop_id: shop.id }).search).to contain_exactly(review_in_shop)
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
