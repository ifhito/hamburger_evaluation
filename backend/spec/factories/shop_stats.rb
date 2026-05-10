FactoryBot.define do
  factory :shop_stat do
    association :shop
    average_rating { 4.0 }
    burger_count { 1 }
    review_count { 1 }
    weighted_score { 4.0 }
    confidence { 0.5 }
  end
end
