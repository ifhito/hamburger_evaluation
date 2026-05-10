module ShopStats
  class RecalculateShopStatService
    def initialize(shop)
      @shop = shop
    end

    def invoke
      stats = burger_stats
      review_count = stats.sum(&:review_count)
      weights = stats.map(&:review_count)

      ShopStat.upsert(
        {
          shop_id:        @shop.id,
          burger_count:   stats.size,
          review_count:   review_count,
          average_rating: weighted_average(stats, weights, &:average_rating),
          weighted_score: weighted_average(stats, weights, &:weighted_score),
          confidence:     weighted_average(stats, weights, &:confidence),
          created_at:     Time.current,
          updated_at:     Time.current
        },
        unique_by: :shop_id,
        update_only: %i[burger_count review_count average_rating weighted_score confidence updated_at],
        record_timestamps: false
      )
    end

    private

    def burger_stats
      @shop.burgers.joins(:burger_stat).includes(:burger_stat).map(&:burger_stat)
    end

    def weighted_average(stats, weights)
      total_weight = weights.sum
      return 0.0 if stats.empty? || total_weight.zero?

      weighted_sum = stats.zip(weights).sum do |stat, weight|
        yield(stat).to_f * weight
      end
      (weighted_sum / total_weight).round(2)
    end
  end
end
