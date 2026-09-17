require "rails_helper"

RSpec.describe Shops::CreateShopService do
  let(:user) { create(:user) }

  it "creates a pending shop owned by the user" do
    param = Shops::CreateParameter.new(name: "Service Shop")

    shop = described_class.new(user: user, params: param).invoke

    expect(shop).to be_persisted
    expect(shop).to be_pending
    expect(shop.creator).to eq(user)
  end

  it "delegates persistence to the repository with the initial status" do
    repository = instance_double(Shops::ShopRepository)
    param = Shops::CreateParameter.new(name: "X")
    allow(repository).to receive(:create!).and_return(:created)

    result = described_class.new(user: user, params: param, repository: repository).invoke

    expect(result).to eq(:created)
    expect(repository).to have_received(:create!).with(creator: user, name: "X", status: "pending")
  end
end
