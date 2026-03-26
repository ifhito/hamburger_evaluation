class Rating
  VALID_RANGE = 1..5

  attr_reader :value

  def initialize(value)
    raise ArgumentError, "Invalid rating: #{value}" unless VALID_RANGE.cover?(value.to_i)
    @value = value.to_i
    freeze
  end

  def excellent? = value >= 4
  def poor?      = value <= 2
  def ==(other)  = other.is_a?(Rating) && value == other.value
  def to_f       = value.to_f
  def to_s       = value.to_s
end
