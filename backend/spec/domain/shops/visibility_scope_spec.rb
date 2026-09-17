require "rails_helper"

RSpec.describe Shops::VisibilityScope do
  describe ".for" do
    it "admin は全 status・owner 制約なし" do
      spec = described_class.for(viewer_id: 1, admin: true)
      expect(spec.statuses).to eq(Shops::ShopStatus::ALL)
      expect(spec.owner_id).to be_nil
    end

    it "匿名は公開 status のみ・owner 制約なし" do
      spec = described_class.for(viewer_id: nil, admin: false)
      expect(spec.statuses).to eq(Shops::ShopStatus::PUBLICLY_LISTED)
      expect(spec.owner_id).to be_nil
    end

    it "一般ユーザーは公開 status + 自分の owner_id" do
      spec = described_class.for(viewer_id: 42, admin: false)
      expect(spec.statuses).to eq(Shops::ShopStatus::PUBLICLY_LISTED)
      expect(spec.owner_id).to eq(42)
    end
  end

  it "初期化後は frozen" do
    expect(described_class.for(viewer_id: nil)).to be_frozen
  end
end
