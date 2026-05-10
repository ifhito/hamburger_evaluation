require "rails_helper"

RSpec.describe Reviews::CreatingReviewRepository do
  let(:shop)       { create(:shop) }
  let(:user)       { create(:user) }
  let(:repository) { described_class.new }

  describe "#create_burger_for_shop" do
    it "店舗内バーガーを作成すること" do
      expect {
        repository.create_burger_for_shop(shop: shop, burger_name: "New Burger")
      }.to change(Burger, :count).by(1)
    end

    it "作成したバーガーを店舗に所属させて返すこと" do
      burger = repository.create_burger_for_shop(shop: shop, burger_name: "New Burger")
      expect(burger.name).to eq("New Burger")
      expect(burger.shop).to eq(shop)
      expect(burger).to be_persisted
    end
  end

  describe "#create_review!" do
    let(:burger) { create(:burger, shop: shop) }

    it "レビューを作成してレビューオブジェクトを返すこと" do
      review = repository.create_review!(user: user, burger: burger, rating: 4, comment: "Great")
      expect(review).to be_persisted
      expect(review.user).to eq(user)
      expect(review.burger).to eq(burger)
      expect(review.rating).to eq(4)
    end
  end
end
