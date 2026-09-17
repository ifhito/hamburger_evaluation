require "rails_helper"

RSpec.describe Users::UserRepository do
  subject(:repository) { described_class.new }

  describe "#kept" do
    it "returns kept users" do
      kept_user = create(:user)
      discarded_user = create(:user)
      discarded_user.discard

      expect(repository.kept).to contain_exactly(kept_user)
    end
  end

  describe "#find_kept!" do
    it "returns a kept user" do
      user = create(:user)

      expect(repository.find_kept!(user.id)).to eq(user)
    end
  end

  describe "#find_kept_by_email" do
    it "returns a kept user by email" do
      user = create(:user, email: "find@example.com")

      expect(repository.find_kept_by_email("find@example.com")).to eq(user)
    end
  end

  describe "#create" do
    it "builds a new user with the given attributes" do
      user = repository.build(username: "new", email: "new@example.com", password: "password123")

      expect(user).to be_a(User)
      expect(user.email).to eq("new@example.com")
    end
  end

  describe "#save" do
    it "persists the user and returns true" do
      user = repository.build(username: "save", email: "save@example.com", password: "password123")

      expect(repository.save(user)).to be(true)
      expect(user).to be_persisted
    end
  end

  describe "#update!" do
    it "updates the user and returns it" do
      user = create(:user, username: "before")

      result = repository.update!(user, username: "after")

      expect(result).to eq(user)
      expect(user.reload.username).to eq("after")
    end
  end

  describe "#discard!" do
    it "soft-deletes the user" do
      user = create(:user)

      repository.discard!(user)

      expect(user.reload.discarded_at).not_to be_nil
    end
  end
end
