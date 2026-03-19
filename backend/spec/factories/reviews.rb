FactoryBot.define do
  factory :review do
    rating  { rand(1..5) }
    comment { Faker::Lorem.sentence }
    association :user
    association :burger
  end
end
