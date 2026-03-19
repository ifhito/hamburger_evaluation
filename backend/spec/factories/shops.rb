FactoryBot.define do
  factory :shop do
    name { Faker::Restaurant.name }
  end
end
