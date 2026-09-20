require "rails_helper"

RSpec.describe UserPolicy, type: :policy do
  subject(:policy) { described_class.new(current_user, target_user) }

  let(:target_user) { create(:user) }
  let(:other_user) { create(:user) }

  context "when the current user is the target user" do
    let(:current_user) { target_user }

    it "permits update" do
      expect(policy.update?).to be(true)
    end

    it "permits destroy" do
      expect(policy.destroy?).to be(true)
    end
  end

  context "when the current user is another user" do
    let(:current_user) { other_user }

    it "denies update" do
      expect(policy.update?).to be(false)
    end

    it "denies destroy" do
      expect(policy.destroy?).to be(false)
    end
  end
end
