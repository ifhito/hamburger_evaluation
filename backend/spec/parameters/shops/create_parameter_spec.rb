require "rails_helper"

RSpec.describe Shops::CreateParameter do
  it "exposes the name" do
    expect(described_class.new(name: "A").name).to eq("A")
  end

  it "raises when name is missing" do
    expect { described_class.new({}) }.to raise_error(Dry::Struct::Error)
  end
end
