// Cloudflare Workers(Static Assets)用。静的ファイルを配信しつつ、
// /api/* を API へ転送する。4 候補で唯一、設定ではなくコードを書く方式。
//
// API のオリジンは環境変数 API_ORIGIN で渡す(wrangler.toml の [vars])。
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname.startsWith("/api/")) {
      // /api を取り除いて API へ渡す(Vite の dev proxy と同じ規則)。
      const target = new URL(url.pathname.replace(/^\/api/, "") || "/", env.API_ORIGIN);
      target.search = url.search;
      // redirect: "manual" が要る。既定では fetch が 3xx を Worker の中で
      // 追いかけてしまい、リダイレクト先の中身を 200 として返す。そうなると
      // ブラウザの URL が変わらず、Google ログインのような 302 を前提にした
      // 流れが壊れる(アプリが知らない /api/... のまま描画され 404 になる)。
      return fetch(new Request(target, request), { redirect: "manual" });
    }
    return env.ASSETS.fetch(request);
  },
};
