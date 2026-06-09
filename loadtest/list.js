import http from "k6/http";
import { check, sleep } from "k6";

// character-api の一覧取得に対する負荷/性能テスト（k6）。
// 実行例: BASE_URL=http://localhost:8080 API_KEY=dev-api-key k6 run loadtest/list.js
const BASE = __ENV.BASE_URL || "http://localhost:8080";
const KEY = __ENV.API_KEY || "dev-api-key";

export const options = {
  stages: [
    { duration: "30s", target: 10 }, // ランプアップ
    { duration: "1m", target: 10 }, // 定常
    { duration: "20s", target: 0 }, // ランプダウン
  ],
  thresholds: {
    // 失敗率 1% 未満、p95 レイテンシ 500ms 未満を SLO とする。
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

const params = { headers: { "X-Internal-API-Key": KEY } };

export default function () {
  const res = http.get(`${BASE}/api/v1/characters?limit=20`, params);
  check(res, {
    "status is 200": (r) => r.status === 200,
    "has items": (r) => {
      try {
        return Array.isArray(JSON.parse(r.body).items);
      } catch {
        return false;
      }
    },
  });
  sleep(1);
}
