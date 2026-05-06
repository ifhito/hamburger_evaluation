require "rails_helper"

RSpec.describe Shops::ShopQuery do
  subject(:query) { described_class.new }

  describe "#search" do
    it "returns shops ordered by name" do
      zeta = create(:shop, name: "Zeta Grill")
      alpha = create(:shop, name: "Alpha Burger")

      expect(query.search).to eq([ alpha, zeta ])
    end

    it "filters shops by keyword case-insensitively" do
      matched = create(:shop, name: "Burger King")
      create(:shop, name: "McDonald's")

      expect(query.search(keyword: "burger")).to contain_exactly(matched)
    end
  end

  describe "#find!" do
    it "returns the shop" do
      shop = create(:shop)

      expect(query.find!(shop.id)).to eq(shop)
    end
  end
end
