module Reviews
  class BurgerScore
    attr_reader :weighted_average, :confidence, :sample_size

    def initialize(weighted_average:, confidence:, sample_size:)
      @weighted_average = weighted_average.round(2)
      @confidence       = confidence.clamp(0.0, 1.0).round(4)
      @sample_size      = sample_size
      freeze
    end

    def reliable? = confidence >= 0.6

    def self.empty
      new(weighted_average: 0.0, confidence: 0.0, sample_size: 0)
    end
  end
end
