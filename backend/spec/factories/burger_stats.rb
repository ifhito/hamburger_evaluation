FactoryBot.define do
  factory :burger_stat do
    association :burger
    average_rating { 0.0 }
    review_count   { 0 }
  end
end
