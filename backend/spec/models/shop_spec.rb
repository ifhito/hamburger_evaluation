require "rails_helper"

RSpec.describe Shop, type: :model do
  describe "validations" do
    it "is invalid without a name" do
      expect(build(:shop, name: nil)).not_to be_valid
    end
  end

  describe "status enum" do
    it "maps pending/active/rejected" do
      expect(Shop.statuses).to eq("pending" => 0, "active" => 1, "rejected" => 2)
    end

    it "defaults to active via the factory" do
      expect(create(:shop)).to be_active
    end
  end

  describe "creator association" do
    it "is optional" do
      expect(build(:shop, creator: nil)).to be_valid
    end

    it "can belong to a creator" do
      user = create(:user)
      expect(create(:shop, creator: user).creator).to eq(user)
    end
  end
end
