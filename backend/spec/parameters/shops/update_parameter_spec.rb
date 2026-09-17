require "rails_helper"

RSpec.describe Shops::UpdateParameter do
  it "allows name to be omitted" do
    expect(described_class.new({}).to_h).to eq({})
  end

  it "exposes the name when provided" do
    expect(described_class.new(name: "B").name).to eq("B")
  end
end
