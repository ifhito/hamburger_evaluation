require "rails_helper"

RSpec.describe Shops::UpdateShopService do
  it "updates the shop name" do
    shop = create(:shop, name: "Old")
    param = Shops::UpdateParameter.new(name: "New")

    described_class.new(shop: shop, params: param).invoke

    expect(shop.reload.name).to eq("New")
  end

  it "leaves the shop unchanged when no attributes are given" do
    shop = create(:shop, name: "Keep")
    param = Shops::UpdateParameter.new({})

    result = described_class.new(shop: shop, params: param).invoke

    expect(result).to eq(shop)
    expect(shop.reload.name).to eq("Keep")
  end
end
