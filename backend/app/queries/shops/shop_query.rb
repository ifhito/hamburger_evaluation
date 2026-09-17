module Shops
  class ShopQuery
    # 公開一覧。可視性の規則は Shops::VisibilityScope(ドメイン)が決め、ここでは SQL に翻訳する。
    def search(keyword: nil, viewer: nil)
      spec  = Shops::VisibilityScope.for(viewer_id: viewer&.id, admin: viewer&.admin?)
      scope = Shop.where(status: spec.statuses)
      scope = scope.or(Shop.where(creator_id: spec.owner_id)) if spec.owner_id
      scope = scope.where("name ILIKE ?", "%#{Shop.sanitize_sql_like(keyword)}%") if keyword.present?
      scope.order(:name)
    end

    # 詳細。閲覧不可なら RecordNotFound(404・情報漏れ防止)。
    def find_visible!(id, viewer: nil)
      shop = Shop.find(id)
      entity = Shops::ShopEntity.from_record(shop)
      raise ActiveRecord::RecordNotFound unless entity.viewable_by?(Users::UserEntity.from_record(viewer))

      shop
    end

    # 全 status を取得(レビュー作成の認可判定・管理用)。
    def find!(id)
      Shop.find(id)
    end
  end
end
