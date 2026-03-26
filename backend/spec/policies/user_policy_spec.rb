require "rails_helper"

RSpec.describe UserPolicy, type: :policy do
  subject { described_class }

  let(:user)       { create(:user) }
  let(:other_user) { create(:user) }

  permissions :update? do
    it "permits the user themselves" do
      expect(subject).to permit(user, user)
    end

    it "denies other users" do
      expect(subject).not_to permit(other_user, user)
    end
  end

  permissions :destroy? do
    it "permits the user themselves" do
      expect(subject).to permit(user, user)
    end

    it "denies other users" do
      expect(subject).not_to permit(other_user, user)
    end
  end
end
