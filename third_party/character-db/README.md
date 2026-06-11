# third_party/character-db （ベンダリングされたスキーマ）

これは **`character-db` リポジトリから取り込んだ（vendoring した）スキーマファイル**である。
以前は git submodule だったが、この API が実際に使うのは下記の数ファイルだけなのに
リポジトリ全体（Dockerfile・CI・infra 等）を展開していて無駄だったため、**必要な
ファイルだけを通常ファイルとしてコミットする方式**に変更した。

唯一の正は依然として `character-db`（`Yu-be-shi/character-db`）であり、ここにある
ファイルは**手で編集しない**。スキーマ追従は `.github/workflows/schema-sync.yml` が
`character-db` の更新通知（`repository_dispatch: character-db-updated`）を受けて自動で
上書きコピー＋ `make sqlc` 再生成＋追従 PR を作る。

## 取り込んでいるファイル

| ファイル | 用途 |
|---|---|
| `schema.sql` | sqlc のスキーマ源（テーブル/ENUM） + 統合テストの DDL |
| `views/00_set_updated_at.sql` | 統合テスト：`updated_at` トリガー |
| `views/10_character_write_functions.sql` | sqlc のカタログ源 + 統合テスト：`update_character` / `soft_delete_character` 等の書き込み関数 |

新たに別のファイルが必要になったら、`sqlc.yaml` / 統合テスト / `schema-sync.yml` の
コピー対象の 3 か所を合わせて更新すること。

## 取り込み元

`SOURCE_SHA` に取り込み元コミット（`Yu-be-shi/character-db`）の SHA を記録する。
手動で更新する場合は schema-sync を `workflow_dispatch` で回すのが正規手順。

> ローカル開発で character-db を未コミットのまま先行修正した場合（例: 予約パターンの
> `confirmed_at` / `creation_token` / `confirm_character` / `gc_unconfirmed_characters` 追加）、
> ここのファイルは手で同期され `SOURCE_SHA` より先行することがある。character-db を
> コミットして schema-sync が回れば `SOURCE_SHA` が実コミットに更新される。
