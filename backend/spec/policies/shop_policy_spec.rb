require "rails_helper"

RSpec.describe ShopPolicy do
  let(:admin) { create(:user, :admin) }
  let(:user)  { create(:user) }

  describe "#create?" do
    it "allows any authenticated user" do
      expect(described_class.new(user, Shop).create?).to be true
    end

    it "denies anonymous users" do
      expect(described_class.new(nil, Shop).create?).to be false
    end
  end

  describe "moderation actions" do
    let(:shop) { create(:shop) }

    it "allows admins" do
      policy = described_class.new(admin, shop)
      expect(policy.update?).to be true
      expect(policy.approve?).to be true
      expect(policy.reject?).to be true
      expect(policy.moderate_index?).to be true
    end

    it "denies non-admins" do
      policy = described_class.new(user, shop)
      expect(policy.update?).to be false
      expect(policy.approve?).to be false
      expect(policy.reject?).to be false
      expect(policy.moderate_index?).to be false
    end
  end

  describe "#review?" do
    it "allows any authenticated user on an active shop" do
      expect(described_class.new(user, create(:shop)).review?).to be true
    end

    it "allows the creator on their pending shop" do
      shop = create(:shop, :pending, creator: user)
      expect(described_class.new(user, shop).review?).to be true
    end

    it "denies a non-owner on a pending shop" do
      shop = create(:shop, :pending, creator: create(:user))
      expect(described_class.new(user, shop).review?).to be false
    end

    it "denies everyone (including admin and creator) on a rejected shop" do
      shop = create(:shop, :rejected, creator: user)
      expect(described_class.new(user, shop).review?).to be false
      expect(described_class.new(admin, shop).review?).to be false
    end
  end
end
