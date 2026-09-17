require "rails_helper"

RSpec.describe Shops::AdminShopQuery do
  subject(:query) { described_class.new }

  describe "#search" do
    it "returns all shops newest first" do
      older = create(:shop, created_at: 2.days.ago)
      newer = create(:shop, :pending, created_at: 1.day.ago)

      expect(query.search).to eq([ newer, older ])
    end

    it "filters by status" do
      pending = create(:shop, :pending)
      create(:shop)

      expect(query.search(status: "pending")).to contain_exactly(pending)
    end
  end
end
