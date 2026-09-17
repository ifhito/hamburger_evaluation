require "rails_helper"

RSpec.describe Users::DeleteUserService do
  describe "#invoke" do
    it "delegates persistence to a repository" do
      user = create(:user)
      repository = instance_double(Users::UserRepository)
      allow(repository).to receive(:discard!).with(user).and_return(true)

      described_class.new(user: user, repository: repository).invoke

      expect(repository).to have_received(:discard!).with(user)
    end
  end
end
