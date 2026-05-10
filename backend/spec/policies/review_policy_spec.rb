require "rails_helper"

RSpec.describe ReviewPolicy, type: :policy do
  subject(:policy) { described_class.new(current_user, review) }

  let(:owner) { create(:user) }
  let(:other_user) { create(:user) }
  let(:review) { create(:review, user: owner) }

  context "when the current user owns the review" do
    let(:current_user) { owner }

    it "permits update" do
      expect(policy.update?).to be(true)
    end

    it "permits destroy" do
      expect(policy.destroy?).to be(true)
    end
  end

  context "when the current user does not own the review" do
    let(:current_user) { other_user }

    it "denies update" do
      expect(policy.update?).to be(false)
    end

    it "denies destroy" do
      expect(policy.destroy?).to be(false)
    end
  end
end
