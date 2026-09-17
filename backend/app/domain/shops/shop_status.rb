module Shops
  class ShopStatus
    PENDING  = "pending"
    ACTIVE   = "active"
    REJECTED = "rejected"

    ALL             = [ PENDING, ACTIVE, REJECTED ].freeze
    PUBLICLY_LISTED = [ ACTIVE ].freeze

    attr_reader :value

    def self.initial = new(PENDING)

    def initialize(value)
      @value = value.to_s
      raise ArgumentError, "Invalid shop status: #{value}" unless ALL.include?(@value)

      freeze
    end

    def pending?  = value == PENDING
    def active?   = value == ACTIVE
    def rejected? = value == REJECTED

    def approve = self.class.new(ACTIVE)
    def reject  = self.class.new(REJECTED)

    def ==(other) = other.is_a?(self.class) && value == other.value
    def to_s = value
  end
end
