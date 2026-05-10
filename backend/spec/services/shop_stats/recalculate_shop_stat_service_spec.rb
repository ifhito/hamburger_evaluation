require "rails_helper"

RSpec.describe ShopStats::RecalculateShopStatService do
  describe "#invoke" do
    it "stores a shop score calculated from burger stats weighted by review count" do
      shop = create(:shop)
      first_burger = create(:burger, shop: shop)
      second_burger = create(:burger, shop: shop)
      create(:burger, shop: shop)

      create(
        :burger_stat,
        burger: first_burger,
        review_count: 10,
        average_rating: 4.8,
        weighted_score: 4.5,
        confidence: 0.9
      )
      create(
        :burger_stat,
        burger: second_burger,
        review_count: 2,
        average_rating: 3.0,
        weighted_score: 3.5,
        confidence: 0.3
      )

      described_class.new(shop).invoke

      stat = ShopStat.find_by!(shop: shop)
      expect(stat.burger_count).to eq(2)
      expect(stat.review_count).to eq(12)
      expect(stat.average_rating).to eq(4.5)
      expect(stat.weighted_score).to eq(4.33)
      expect(stat.confidence).to eq(0.8)
    end

    it "stores zero scores when the shop has no scored burgers" do
      shop = create(:shop)

      described_class.new(shop).invoke

      stat = ShopStat.find_by!(shop: shop)
      expect(stat.burger_count).to eq(0)
      expect(stat.review_count).to eq(0)
      expect(stat.average_rating).to eq(0.0)
      expect(stat.weighted_score).to eq(0.0)
      expect(stat.confidence).to eq(0.0)
    end
  end
end
