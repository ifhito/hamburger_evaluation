require "rails_helper"

RSpec.describe Reviews::CreatingReviewRepository do
  let(:shop)       { create(:shop) }
  let(:user)       { create(:user) }
  let(:repository) { described_class.new }

  describe "#create_burger_for_shop" do
    it "バーガーを作成して店舗と紐付けること" do
      expect {
        repository.create_burger_for_shop(shop: shop, burger_name: "New Burger")
      }.to change(Burger, :count).by(1)
        .and change(ShopsAndBurger, :count).by(1)
    end

    it "作成したバーガーを返すこと" do
      burger = repository.create_burger_for_shop(shop: shop, burger_name: "New Burger")
      expect(burger.name).to eq("New Burger")
      expect(burger).to be_persisted
    end
  end

  describe "#create_review!" do
    let(:burger) { create(:burger).tap { |b| ShopsAndBurger.create!(shop: shop, burger: b) } }

    it "レビューを作成してレビューオブジェクトを返すこと" do
      review = repository.create_review!(user: user, burger: burger, rating: 4, comment: "Great")
      expect(review).to be_persisted
      expect(review.user).to eq(user)
      expect(review.burger).to eq(burger)
      expect(review.rating).to eq(4)
    end
  end
end
