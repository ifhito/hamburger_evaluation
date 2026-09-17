module Burgers
  class BurgerEntity
    attr_reader :id

    # review_facts: Reviews::ReviewFact の配列
    def initialize(id:, review_facts:)
      @id           = id
      @review_facts = review_facts
    end

    def review_count = @review_facts.size

    def average_rating
      ratings = @review_facts.map(&:rating)
      return 0.0 if ratings.empty?

      (ratings.sum / ratings.size).round(2)
    end

    # 重み付きスコア/信頼度の算出は既存のドメインサービスへ委譲
    def score(calculator: Reviews::BurgerScoreCalculator.new)
      calculator.call(@review_facts)
    end
  end
end
