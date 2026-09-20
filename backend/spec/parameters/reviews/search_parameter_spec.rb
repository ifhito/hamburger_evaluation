require "rails_helper"

RSpec.describe Reviews::SearchParameter do
  describe "attribute coercion" do
    it "coerces string rating to integer" do
      param = described_class.new(rating: "5")
      expect(param.rating).to eq(5)
    end

    it "coerces string shop_id to integer" do
      param = described_class.new(shop_id: "3")
      expect(param.shop_id).to eq(3)
    end
  end

  describe "optional attributes" do
    it "allows all attributes to be omitted" do
      param = described_class.new
      expect(param.rating).to be_nil
      expect(param.keyword).to be_nil
      expect(param.shop_id).to be_nil
    end

    it "allows partial attributes" do
      param = described_class.new(keyword: "crispy")
      expect(param.keyword).to eq("crispy")
      expect(param.rating).to be_nil
    end
  end
end
