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

  describe "#search with viewer" do
    it "shows only active shops to anonymous viewers" do
      active = create(:shop)
      create(:shop, :pending)

      expect(query.search(viewer: nil)).to contain_exactly(active)
    end

    it "also shows the viewer's own pending shops" do
      user = create(:user)
      active = create(:shop)
      mine = create(:shop, :pending, creator: user)
      create(:shop, :pending)

      expect(query.search(viewer: user)).to contain_exactly(active, mine)
    end

    it "shows all shops to admins" do
      admin = create(:user, :admin)
      active = create(:shop)
      pending = create(:shop, :pending)
      rejected = create(:shop, :rejected)

      expect(query.search(viewer: admin)).to contain_exactly(active, pending, rejected)
    end
  end

  describe "#find_visible!" do
    it "returns an active shop to anyone" do
      shop = create(:shop)

      expect(query.find_visible!(shop.id)).to eq(shop)
    end

    it "raises for a pending shop viewed by a non-owner" do
      shop = create(:shop, :pending, creator: create(:user))

      expect { query.find_visible!(shop.id, viewer: create(:user)) }
        .to raise_error(ActiveRecord::RecordNotFound)
    end

    it "returns a pending shop to its creator" do
      owner = create(:user)
      shop = create(:shop, :pending, creator: owner)

      expect(query.find_visible!(shop.id, viewer: owner)).to eq(shop)
    end
  end

  describe "#find!" do
    it "returns the shop" do
      shop = create(:shop)

      expect(query.find!(shop.id)).to eq(shop)
    end
  end
end
