FactoryBot.define do
  factory :burger do
    name { Faker::Food.dish }
  end
end
