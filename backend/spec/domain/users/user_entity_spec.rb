require "rails_helper"

RSpec.describe Users::UserEntity do
  describe ".from_record" do
    it "record が nil なら nil を返す" do
      expect(described_class.from_record(nil)).to be_nil
    end

    it "record の属性からエンティティを構築する" do
      entity = described_class.from_record(Struct.new(:id, :admin).new(7, true))
      expect(entity.id).to eq(7)
      expect(entity.admin?).to be true
    end
  end

  describe "#admin? / #can_moderate_shops?" do
    it "admin が true なら true" do
      entity = described_class.new(id: 1, admin: true)
      expect(entity.admin?).to be true
      expect(entity.can_moderate_shops?).to be true
    end

    it "admin が false なら false" do
      entity = described_class.new(id: 1, admin: false)
      expect(entity.admin?).to be false
      expect(entity.can_moderate_shops?).to be false
    end
  end

  describe "#manages?" do
    it "自分の id と一致すれば true" do
      expect(described_class.new(id: 1, admin: false).manages?(1)).to be true
    end

    it "異なる id なら false" do
      expect(described_class.new(id: 1, admin: false).manages?(2)).to be false
    end
  end

  it "初期化後は frozen" do
    expect(described_class.new(id: 1, admin: false)).to be_frozen
  end
end
