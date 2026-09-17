module Shops
  class CreateParameter < Dry::Struct
    module Types
      include Dry.Types()
    end

    attribute :name, Types::String
  end
end
