require "rails_helper"

RSpec.describe Shops::ShopRepository do
  subject(:repository) { described_class.new }

  let(:user) { create(:user) }

  describe "#create!" do
    it "creates a shop with creator, name and status" do
      shop = repository.create!(creator: user, name: "Repo Shop", status: "pending")

      expect(shop).to be_persisted
      expect(shop.creator).to eq(user)
      expect(shop.name).to eq("Repo Shop")
      expect(shop).to be_pending
    end
  end

  describe "#find!" do
    it "returns the shop" do
      shop = create(:shop)

      expect(repository.find!(shop.id)).to eq(shop)
    end
  end

  describe "#update!" do
    it "updates attributes" do
      shop = create(:shop, name: "Old")

      repository.update!(shop, name: "New")

      expect(shop.reload.name).to eq("New")
    end
  end

  describe "#update_status!" do
    it "updates status and moderation note" do
      shop = create(:shop, :pending)

      repository.update_status!(shop, status: "rejected", moderation_note: "bad")

      expect(shop.reload).to be_rejected
      expect(shop.moderation_note).to eq("bad")
    end
  end
end
