require "rails_helper"

RSpec.describe User, type: :model do
  describe "associations" do
    it { is_expected.to have_many(:reviews).dependent(:destroy) }
  end

  describe "has_secure_password" do
    it "saves a password_digest" do
      user = build(:user, password: "secret123", password_confirmation: "secret123")
      user.save!
      expect(user.password_digest).to be_present
    end

    it "authenticates with the correct password" do
      user = create(:user, password: "secret123", password_confirmation: "secret123")
      expect(user.authenticate("secret123")).to eq(user)
    end

    it "returns false for an incorrect password" do
      user = create(:user, password: "secret123", password_confirmation: "secret123")
      expect(user.authenticate("wrong")).to be_falsy
    end
  end

  describe "soft delete (discard)" do
    it "sets discarded_at when discarded" do
      user = create(:user)
      expect { user.discard }.to change { user.discarded_at }.from(nil)
    end

    it "excludes discarded users from User.kept" do
      user = create(:user)
      user.discard
      expect(User.kept).not_to include(user)
    end

    it "includes active users in User.kept" do
      user = create(:user)
      expect(User.kept).to include(user)
    end
  end

  describe "validations" do
    it "is invalid without an email" do
      user = build(:user, email: nil)
      expect(user).not_to be_valid
    end

    it "is invalid with a duplicate email" do
      existing = create(:user)
      duplicate = build(:user, email: existing.email)
      expect(duplicate).not_to be_valid
    end
  end
end
