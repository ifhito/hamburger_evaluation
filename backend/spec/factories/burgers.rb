FactoryBot.define do
  factory :burger do
    name { Faker::Food.dish }
    association :shop
  end
end
