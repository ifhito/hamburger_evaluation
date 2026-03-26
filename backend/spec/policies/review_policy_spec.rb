require "rails_helper"

RSpec.describe ReviewPolicy, type: :policy do
  subject { described_class }

  let(:user)       { create(:user) }
  let(:other_user) { create(:user) }
  let(:review)     { create(:review, user: user) }

  permissions :update? do
    it "permits the review owner" do
      expect(subject).to permit(user, review)
    end

    it "denies other users" do
      expect(subject).not_to permit(other_user, review)
    end
  end

  permissions :destroy? do
    it "permits the review owner" do
      expect(subject).to permit(user, review)
    end

    it "denies other users" do
      expect(subject).not_to permit(other_user, review)
    end
  end
end
