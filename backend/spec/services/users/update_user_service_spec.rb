require "rails_helper"

RSpec.describe Users::UpdateUserService do
  describe "#invoke" do
    it "delegates persistence to a repository" do
      user = create(:user)
      params = { username: "updated" }
      repository = instance_double(Users::UserRepository)
      allow(repository).to receive(:update!).with(user, username: "updated").and_return(user)

      result = described_class.new(user: user, params: params, repository: repository).invoke

      expect(result).to eq(user)
      expect(repository).to have_received(:update!).with(user, username: "updated")
    end
  end
end
