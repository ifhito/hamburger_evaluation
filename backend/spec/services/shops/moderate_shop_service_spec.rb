require "rails_helper"

RSpec.describe Shops::ModerateShopService do
  describe "#approve" do
    it "activates the shop and clears the moderation note" do
      shop = create(:shop, :rejected, moderation_note: "old")

      result = described_class.new(shop: shop).approve

      expect(result.reload).to be_active
      expect(result.moderation_note).to be_nil
    end
  end

  describe "#reject" do
    it "rejects the shop with a moderation note" do
      shop = create(:shop, :pending)

      result = described_class.new(shop: shop).reject(moderation_note: "理由")

      expect(result.reload).to be_rejected
      expect(result.moderation_note).to eq("理由")
    end
  end
end
